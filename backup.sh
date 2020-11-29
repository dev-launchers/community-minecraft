#!/bin/bash
eval `ssh-agent -s`
mkdir /root/.ssh
cp /root/ssh-config/known_hosts /root/.ssh/known_hosts
cp /root/ssh-config/authorized_keys /root/.ssh/authorized_keys
cp /root/ssh-config/id_ed25519 /root/.ssh/id_ed25519
chmod 0600 /root/.ssh/id_ed25519
ssh-add /root/.ssh/id_ed25519

cd /minecraft/community-minecraft/minecraft_home
git remote set-url origin git@github.com:dev-launchers-sandbox/community-minecraft.git
git config --global user.email "team@devlaunchers.com"
git config --global user.name "dev-launchers-backup"
while true; do
echo "Preparing backup"
git checkout cron-backup
git add sever
git commit -m "Backup ${date}"
git push -f origin cron-backup
sleep ${BACKUP_FREQ}
done; 