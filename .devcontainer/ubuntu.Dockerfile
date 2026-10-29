FROM docker.io/ubuntu:23.04
# https://github.com/kncept-oauth/simple-oidc
# DEBUGGING: docker build -f .devcontainer/ubuntu.Dockerfile -t ubuntu-dev . && docker run -it ubuntu-dev bash

# Consider lscr.io/linuxserver/code-server:latest

RUN apt-get update
RUN apt-get -y upgrade
RUN apt-get -y install sudo wget curl vim git unzip bash

# Install Go
RUN \
    wget -q https://go.dev/dl/go1.22.0.linux-amd64.tar.gz -O /tmp/go.tar.gz && \
    rm -rf /usr/local/go && \
    tar -C /usr/local -xzf /tmp/go.tar.gz && \
    rm /tmp/go.tar.gz
ENV PATH="${PATH}:/usr/local/go/bin"

# Install Opencode
RUN \
    curl -fsSL https://raw.githubusercontent.com/opencode-ai/opencode/refs/heads/main/install | bash

# User
RUN usermod -aG sudo ubuntu
RUN echo "ubuntu:ubuntu" | chpasswd

RUN chown -R ubuntu:ubuntu /go

USER ubuntu
WORKDIR /home/ubuntu

# Per User: disable gradle daemon
# https://docs.gradle.org/7.5.1/userguide/build_environment.html#sec:configuring_jvm_memory
# RUN \
#     mkdir -p /home/ubuntu/.gradle && echo "org.gradle.daemon=false" >> /home/ubuntu/.gradle/gradle.properties && \
#     export GRADLE_OPTS="-Dorg.gradle.daemon=false" && \
#     export JAVA_OPTS="-Xmx2g -XX:+HeapDumpOnOutOfMemoryError -Dfile.encoding=UTF-8" && \
#     echo "org.gradle.jvmargs=${JAVA_OPTS}" >> /home/ubuntu/.gradle/gradle.properties

# Install NVM and node tools
# ARG NODE_VERSION=18
# RUN \
#     curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.3/install.sh | bash
# RUN bash -c 'source .nvm/nvm.sh && npm install -g ts-node'
# known location for the current node version bin tools
# RUN ln -s /home/ubuntu/.nvm/versions/node/v${NODE_VERSION}* /home/ubuntu/.local_node