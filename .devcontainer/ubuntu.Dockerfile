FROM docker.io/ubuntu:23.04
# https://github.com/kncept-oauth/simple-oidc
# DEBUGGING: docker build -f .devcontainer/ubuntu.Dockerfile -t ubuntu-dev . && docker run -it ubuntu-dev bash

# Consider lscr.io/linuxserver/code-server:latest

RUN apt-get update
RUN apt-get -y upgrade
RUN apt-get -y install sudo wget curl vim git unzip bash

ARG GO_VERSION=1.27.3
# Install Go
RUN \
    GO_ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/') && \
    wget -q https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz -O /tmp/go.tar.gz && \
    rm -rf /usr/local/go && \
    tar -C /usr/local -xzf /tmp/go.tar.gz && \
    rm /tmp/go.tar.gz && \
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
