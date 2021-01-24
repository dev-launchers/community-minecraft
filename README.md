# community-minecraft

Repository for Mincraft build by the Dev Launchers community

# commands

apt update -y
apt install openssh-server -y
apt-get install git openjdk-8-jre-headless -y
apt-get install git -y
(set workdir to minecraft_home/server/build)
git config --global --unset core.autocrlf
java -jar BuildTools.jar
(end)

## We're making something amazing! Come help us build it in our [DISCORD](https://discord.io/devlaunchers)!


## Deployment
We use kustomize to construct the kubernetes manifest files and flux to manage deployment.
Staging and production shares the same service file in `./workload/service.yaml`.

### Production
The statefulset definition in under production directory `./production/statefulset.yaml`.

### Staging
The statefulset definition in under staging directory `./staging/statefulset.yaml`.
