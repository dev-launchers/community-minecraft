FROM ubuntu:18.04
RUN apt update -y && apt install -y openssh-server default-jdk wget git
COPY minecraft_home/server /minecraft/community-minecraft/minecraft_home/server
RUN echo "eula=true" > eula.txt
COPY entrypoint.sh /minecraft/entrypoint.sh
COPY backup.sh /minecraft/backup.sh
ENTRYPOINT [ "/minecraft/entrypoint.sh" ]