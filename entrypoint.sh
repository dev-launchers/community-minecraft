#!/bin/bash
service ssh restart

/minecraft/community-minecraft/backup.sh &> /minecraft/community-minecraft/backup.log &

# go to /minecraft/community-minecraft/minecraft_home/server/ because eula.txt and start.sh needs to be in the same directory
cd /minecraft/community-minecraft/minecraft_home/server/
/minecraft/community-minecraft/minecraft_home/server/start.sh > /minecraft/community-minecraft/server.log &
