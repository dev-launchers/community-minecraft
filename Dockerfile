FROM ubuntu:18.04
WORKDIR /minecraft
COPY entrypoint.sh .
RUN apt update -y && apt install -y openssh-server default-jdk wget git
RUN git clone https://github.com/dev-launchers-sandbox/community-minecraft.git
WORKDIR /minecraft/community-minecraft/minecraft_home/sever/
RUN echo "eula=true" > eula.txt
ENTRYPOINT [ "/minecraft/entrypoint.sh" ]
