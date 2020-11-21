FROM ubuntu:18.04
WORKDIR /minecraft
RUN apt update -y && apt install -y openssh-server  default-jdk wget
RUN wget -O /minecraft/minecraft_server.jar https://launcher.mojang.com/v1/objects/35139deedbd5182953cf1caa23835da59ca3d7cd/server.jar
RUN echo "eula=true" > eula.txt
COPY entrypoint.sh .
ENTRYPOINT [ "/minecraft/entrypoint.sh" ]