#!/bin/sh
cp /root/ssh-config/known_hosts /root/.ssh/known_hosts
cp /root/ssh-config/authorized_keys /root/.ssh/authorized_keys
cp /root/ssh-config/id_ed25519 /root/.ssh/id_ed25519
chmod 0600 /root/.ssh/id_ed25519
keychain /root/.ssh/id_ed25519
service ssh start