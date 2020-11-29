FROM ubuntu:18.04
RUN apt update -y && apt install -y openssh-server default-jdk wget git vim
COPY . /minecraft/community-minecraft/
RUN echo "eula=true" > /minecraft/community-minecraft/minecraft_home/server/eula.txt
ENTRYPOINT [ "/minecraft/community-minecraft/entrypoint.sh" ]