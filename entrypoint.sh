#!/bin/bash
service ssh restart
java -Xmx1024M -Xms1024M -jar community-minecraft/minecraft_home/sever/Spigot.jar nogui
