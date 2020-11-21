FROM ubuntu:18.04
WORKDIR /minecraft
RUN apt update -y && apt install -y openssh-server default-jdk wget git
RUN git clone https://github.com/dev-launchers-sandbox/community-minecraft.git
WORKDIR /minecraft/community-minecraft/minecraft_home/sever/
RUN echo "eula=true" > eula.txt
COPY entrypoint.sh .
ENTRYPOINT [ "/minecraft/entrypoint.sh" ]
