#!/bin/bash
service ssh restart

/minecraft/backup.sh &> /minecraft/backup.txt &

/minecraft/community-minecraft/minecraft_home/sever/start.sh