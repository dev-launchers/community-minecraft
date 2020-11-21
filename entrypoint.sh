#!/bin/bash
service ssh restart
java -Xmx1024M -Xms1024M -jar minecraft_server.jar nogui
