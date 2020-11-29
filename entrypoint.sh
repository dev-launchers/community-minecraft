#!/bin/bash
service ssh restart

cp /root/ssh-config/known_hosts /root/.ssh/known_hosts
cp /root/ssh-config/authorized_keys /root/.ssh/authorized_keys
cp /root/ssh-config/id_ed25519 /root/.ssh/id_ed25519
chmod 0600 /root/.ssh/id_ed25519

/minecraft/community-minecraft/minecraft_home/sever/startup.sh &> server_log.txt &

cd /minecraft/community-minecraft/minecraft_home
git remote set-url origin git@github.com:dev-launchers-sandbox/community-minecraft.git
while true; do
git add sever
git checkout -b cron-backup
git commit -m "Backup ${date}"
git push origin cron-backup
sleep ${BACKUP_FREQ}
done; 