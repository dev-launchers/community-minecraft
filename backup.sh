#!/bin/bash
cd /minecraft/community-minecraft/minecraft_home
git remote set-url origin git@github.com:dev-launchers-sandbox/community-minecraft.git
git config --global user.email "team@devlaunchers.com"
git config --global user.name "dev-launchers-backup"
while true; do
echo "Preparing backup"
git add sever
git checkout -b cron-backup
git commit -m "Backup ${date}"
git push -f origin cron-backup
sleep ${BACKUP_FREQ}
done; 