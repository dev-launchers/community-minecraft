FROM ubuntu:20.04
ENV TZ=America/Chicago
RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone
RUN apt update -y && apt install -y openssh-server keychain default-jdk wget git vim
RUN mkdir /root/.ssh
WORKDIR /build
RUN wget https://golang.org/dl/go1.15.6.linux-amd64.tar.gz
RUN tar -xvf go1.15.6.linux-amd64.tar.gz
RUN mv go /usr/local
RUN git config --global user.email "team@devlaunchers.com"
RUN git config --global user.name "dev-launchers-backup"
COPY src /build
RUN /usr/local/go/bin/go build -o service-manager main.go
RUN mv service-manager /usr/local/bin
WORKDIR /minecraft/community-minecraft/
ENTRYPOINT [ "service-manager" ] 