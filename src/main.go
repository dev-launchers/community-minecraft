package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	config, err := loadConfig()
	if err != nil {
		log.Fatalf("Failed to load config, err: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := startSSHAgent(ctx, config); err != nil {
		log.Fatalf("Failed to start ssh agent, err: %v", err)
	}
	// Directory to fetch world data
	if err := os.Chdir(config.workDir); err != nil {
		log.Fatalf("Failed to change working directory to %s, err: %v", config.workDir, err)
	}
	if err := setupWorldData(ctx, config); err != nil {
		log.Fatalf("Failed to fetch latest minecraft data, err: %v", err)
	}

	metrics := newMetrics()

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		runServer(ctx, config, metrics)
	}()
	go func() {
		defer wg.Done()
		backup(ctx, config, metrics)
	}()
	go func() {
		defer wg.Done()
		err := serverMetrics(config, metrics)
		log.Printf("Failed to serve metrics, err: %v", err)
	}()

	waitForShutdown(cancel)
	wg.Wait()
}

func executeCmd(cmd *exec.Cmd, name string) error {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("Failed to get stdout, err: %v", err)
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("Failed to get stderr, err: %v", err)
		return err
	}
	err = cmd.Start()
	if err != nil {
		log.Printf("Failed to start command %s, err: %v", name, err)
		return err
	}
	streamLog(stdout, stderr, name)
	err = cmd.Wait()
	if err != nil {
		log.Printf("Command %s exit with err: %v", name, err)
		return err
	}
	return nil
}

func streamLog(stdout, stderr io.ReadCloser, cmdName string) {
	go func() {
		r := bufio.NewReader(stdout)
		for {
			line, _, err := r.ReadLine()
			if err != nil {
				return
			}
			fmt.Printf("%s stdout: %s\n", cmdName, string(line))
		}
	}()
	go func() {
		r := bufio.NewReader(stderr)
		for {
			line, _, err := r.ReadLine()
			if err != nil {
				return
			}
			fmt.Printf("%s stderr: %s\n", cmdName, string(line))
		}
	}()

}

func startSSHAgent(ctx context.Context, config *config) error {
	return executeCmd(exec.CommandContext(ctx, config.sshScript), "start ssh agent")
}

func setupWorldData(ctx context.Context, config *config) error {
	// Clone world data and plugins
	if err := executeCmd(exec.CommandContext(ctx, "git", "clone", "--recurse-submodules", "-j", "4", config.worldDataRepo), "fetch world data"); err != nil {
		return err
	}
	// Directory to run the server
	serverDir := fmt.Sprintf("%s/community-minecraft-data/server", config.workDir)
	if err := os.Chdir(serverDir); err != nil {
		return err
	}
	// Set remote url so we can auth via ssh key
	err := executeCmd(exec.CommandContext(ctx, "git", "remote", "set-url", "origin", config.worldDataRepo), "set remote url")
	if err != nil {
		return err
	}
	return nil
}

func runServer(ctx context.Context, config *config, metrics *metrics) {
	for {
		if err := executeCmd(exec.CommandContext(ctx, config.startScript, config.serverJar), "run world server"); err != nil {
			metrics.serverErrs.Inc()
		}
		time.Sleep(time.Second * 30)
	}
}

func backup(ctx context.Context, config *config, metrics *metrics) {
	backupTicker := time.NewTicker(config.backupFreq)
	var shutdown bool
	for {
		select {
		case <-ctx.Done():
			log.Println("Backup before termination")
			shutdown = true
		case <-backupTicker.C:
		}
		if err := executeCmd(exec.Command("git", "add", "."), "failed to add world data"); err != nil {
			metrics.backupErrs.WithLabelValues("add").Inc()
			continue
		}
		if err := executeCmd(exec.Command("git", "commit", "-m", time.Now().String()), "run world server"); err != nil {
			metrics.backupErrs.WithLabelValues("commit").Inc()
			continue
		}
		if err := executeCmd(exec.Command("git", "push", "origin", "main"), "run world server"); err != nil {
			metrics.backupErrs.WithLabelValues("push").Inc()
			continue
		}
		log.Println("Backup successfully")
		metrics.updateLastbackup()
		if shutdown {
			return
		}
	}
}

type metrics struct {
	serverErrs prometheus.Counter
	backupErrs *prometheus.CounterVec

	lock       *sync.RWMutex
	lastBackup time.Time
}

func (m *metrics) updateLastbackup() {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.lastBackup = time.Now()
}

func (m *metrics) lastBackupHandler(w http.ResponseWriter, r *http.Request) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	w.Write([]byte(m.lastBackup.String()))
}

func newMetrics() *metrics {
	return &metrics{
		serverErrs: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "community_minecraft",
				Name:      "server_errors",
				Help:      "Count of errors in staring minecraft server",
			},
		),
		backupErrs: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "community_minecraft",
				Name:      "backup_errors",
				Help:      "Count of errors during backup",
			},
			[]string{"cmd"},
		),
		lock: &sync.RWMutex{},
	}
}

func serverMetrics(config *config, metrics *metrics) error {
	pingService := &pingService{
		addr: config.minecraftPort,
	}
	router := http.NewServeMux()
	router.Handle("/status", http.HandlerFunc(pingService.statusHandler))
	router.Handle("/metrics", promhttp.Handler())
	router.Handle("/lastbackup", http.HandlerFunc(metrics.lastBackupHandler))
	server := http.Server{
		Addr:    fmt.Sprintf(":%s", config.metricsPort),
		Handler: router,
	}
	return server.ListenAndServe()
}

type pingService struct {
	addr string
}

func (ps *pingService) statusHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := net.Listen("tcp", ps.addr)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	defer conn.Close()
	w.Write([]byte("healthy"))
}

func waitForShutdown(cancelFunc context.CancelFunc) {
	signals := make(chan os.Signal, 10)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(signals)

	s := <-signals
	log.Printf("Receive signal %s, waiting to shutdown\n", s)
	cancelFunc()
}

type config struct {
	worldDataRepo string
	sshScript     string
	startScript   string
	serverJar     string
	backupFreq    time.Duration
	workDir       string
	metricsPort   string
	minecraftPort string
}

func loadConfig() (*config, error) {
	worldDataRepo, err := requireStr("WORLD_DATA_REPO")
	if err != nil {
		return nil, err
	}
	sshScript, err := requireStr("SSH_SCRIPT")
	if err != nil {
		return nil, err
	}
	startScript, err := requireStr("START_SCRIPT")
	if err != nil {
		return nil, err
	}
	serverJar, err := requireStr("SERVER_JAR")
	if err != nil {
		return nil, err
	}
	backupFreq, err := requireDuration("BACKUP_FREQ")
	if err != nil {
		return nil, err
	}
	workDir, err := requireStr("WORK_DIR")
	if err != nil {
		return nil, err
	}
	metricsPort, err := requireStr("METRICS_PORT")
	if err != nil {
		return nil, err
	}
	minecraftPort, err := requireStr("MINECRAFT_PORT")
	if err != nil {
		return nil, err
	}
	return &config{
		worldDataRepo: worldDataRepo,
		sshScript:     sshScript,
		startScript:   startScript,
		serverJar:     serverJar,
		backupFreq:    backupFreq,
		workDir:       workDir,
		metricsPort:   metricsPort,
		minecraftPort: minecraftPort,
	}, nil
}

func requireStr(envName string) (string, error) {
	envVal := os.Getenv(envName)
	if envVal == "" {
		return "", fmt.Errorf("%s not specified in env var", envName)
	}
	return envVal, nil
}

func requireDuration(envName string) (time.Duration, error) {
	envVal := os.Getenv(envName)
	if envVal == "" {
		return 0, fmt.Errorf("%s not specified in env var", envName)
	}
	return time.ParseDuration(envVal)
}
