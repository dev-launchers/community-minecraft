#!/bin/bash
service ssh restart
java -Xmx1024M -Xms1024M -jar Spigot.jar nogui
