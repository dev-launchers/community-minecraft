#!/bin/bash
service ssh restart

eval `ssh-agent -s`
mkdir /root/.ssh
cp /root/ssh-config/known_hosts /root/.ssh/known_hosts
cp /root/ssh-config/authorized_keys /root/.ssh/authorized_keys
cp /root/ssh-config/id_ed25519 /root/.ssh/id_ed25519
chmod 0600 /root/.ssh/id_ed25519
ssh-add /root/.ssh/id_ed25519

/minecraft/backup.sh &> /minecraft/backup.txt &

/minecraft/community-minecraft/minecraft_home/sever/start.sh