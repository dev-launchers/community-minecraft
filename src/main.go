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
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	config, err := loadConfig()
	if err != nil {
		log.Fatalf("Failed to load config, err: %v", err)
	}
	ctx := context.Background()
	if err := startSSHAgent(ctx, config); err != nil {
		log.Fatalf("Failed to start ssh agent, err: %v", err)
	}
	// Directory to fetch world data
	if err := os.Chdir(config.workDir); err != nil {
		log.Fatalf("Failed to change working directory to %s, err: %v", config.workDir, err)
	}

	// Set remote url so we can auth via ssh key
	if err := executeCmd(exec.CommandContext(ctx, "git", "remote", "set-url", "origin", config.worldDataRepo), "set remote url"); err != nil {
		log.Fatalf("Failed to set remote URL, err: %v", err)
	}

	metrics := newMetrics()

	errGroup, ctx := errgroup.WithContext(ctx)
	errGroup.Go(func() error {
		return runServer(ctx, config, metrics)
	})
	errGroup.Go(func() error {
		return updatePlugin(ctx, config)
	})
	errGroup.Go(func() error {
		return serverMetrics(config, metrics)
	})
	errGroup.Go(func() error {
		return waitForShutdown()
	})

	if !config.disableBackup {
		errGroup.Go(func() error {
			return backup(ctx, config, metrics)
		})
	}

	log.Printf("Terminating, reason: %v\n", errGroup.Wait())
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

func updatePlugin(ctx context.Context, config *config) error {
	var (
		remoteBranch = fmt.Sprintf("origin/%s", config.pluginBranch)
		pluginsDir   = filepath.Join(config.workDir, "plugins")
	)

	ticker := time.NewTicker(config.checkNewPluginFreq)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			// https://stackoverflow.com/questions/3258243/check-if-pull-needed-in-git
			// @ refers to the current branch
			localCommitCmd := exec.CommandContext(ctx, "git", "rev-parse", "@")
			localCommitCmd.Dir = pluginsDir
			localCommitID, err := localCommitCmd.Output()
			if err != nil {
				log.Printf("Failed to get local commit ID, err: %v", err)
				continue
			}
			log.Printf("local commit ID %s", string(localCommitID))

			upstreamCommitCmd := exec.CommandContext(ctx, "git", "rev-parse", remoteBranch)
			upstreamCommitCmd.Dir = pluginsDir
			upstreamCommitID, err := upstreamCommitCmd.Output()
			if err != nil {
				log.Printf("Failed to get upstream commit ID, err: %v", err)
				continue
			}
			log.Printf("upstream commit ID %s", string(upstreamCommitID))

			if string(localCommitID) == string(upstreamCommitID) {
				continue
			}
			// update plugins
			updatePluginCommand := exec.CommandContext(ctx, "git", "pull", "origin", config.pluginBranch)
			updatePluginCommand.Dir = pluginsDir
			if err := executeCmd(updatePluginCommand, "update submodules"); err != nil {
				log.Printf("Failed to pull latest plugin", err)
				continue
			}
		}
	}
}

func runServer(ctx context.Context, config *config, metrics *metrics) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if err := executeCmd(exec.CommandContext(ctx, config.startScript, config.serverJar), "run world server"); err != nil {
			metrics.serverErrs.Inc()
		}
		time.Sleep(time.Second * 30)
	}
}

func backup(ctx context.Context, config *config, metrics *metrics) error {
	backupTicker := time.NewTicker(config.backupFreq)
	var shutdown bool
	for {
		select {
		case <-ctx.Done():
			log.Println("Backup before termination")
			shutdown = true
		case <-backupTicker.C:
		}
		if err := executeCmd(exec.Command("tar", "-czf", "server.tar.gz", config.workDir), "failed to add world data"); err != nil {
			metrics.backupErrs.WithLabelValues("compress").Inc()
			continue
		}
		if err := executeCmd(exec.Command("git", "add", "server.tar.gz"), "failed to add world data"); err != nil {
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
			return nil
		}
	}
}

type metrics struct {
	serverErrs      prometheus.Counter
	checkPluginsErr prometheus.Counter
	backupErrs      *prometheus.CounterVec

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
	const (
		namespace = "community_minecraft"
	)
	return &metrics{
		serverErrs: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "server_errors",
				Help:      "Count of errors in staring minecraft server",
			},
		),
		checkPluginsErr: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "check_plugins_errors",
				Help:      "Count of errors in checking new plugins",
			},
		),
		backupErrs: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
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
	conn, err := net.Dial("tcp", ps.addr)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	defer conn.Close()
	w.Write([]byte("healthy"))
}

func waitForShutdown() error {
	signals := make(chan os.Signal, 10)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(signals)

	s := <-signals
	err := fmt.Errorf("Receive signal %s, waiting to shutdown", s)
	log.Println(err)
	return err
}

type config struct {
	worldDataRepo      string
	sshScript          string
	startScript        string
	serverJar          string
	pluginBranch       string
	checkNewPluginFreq time.Duration
	backupFreq         time.Duration
	disableBackup      bool
	workDir            string
	metricsPort        string
	minecraftPort      string
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
	pluginBranch, err := requireStr("PLUGIN_BRANCH")
	if err != nil {
		return nil, err
	}
	checkNewPluginFreq, err := requireDuration("CHECK_NEW_PLUGIN_FREQ")
	if err != nil {
		return nil, err
	}
	backupFreq, err := requireDuration("BACKUP_FREQ")
	if err != nil {
		return nil, err
	}
	disableBackup, err := readBool("DISABLE_BACKUP")
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
		worldDataRepo:      worldDataRepo,
		sshScript:          sshScript,
		startScript:        startScript,
		serverJar:          serverJar,
		pluginBranch:       pluginBranch,
		checkNewPluginFreq: checkNewPluginFreq,
		backupFreq:         backupFreq,
		disableBackup:      disableBackup,
		workDir:            workDir,
		metricsPort:        metricsPort,
		minecraftPort:      minecraftPort,
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

func readBool(envName string) (bool, error) {
	envVal := os.Getenv(envName)
	if envVal == "" {
		return false, nil
	}
	return strconv.ParseBool(envVal)
}
