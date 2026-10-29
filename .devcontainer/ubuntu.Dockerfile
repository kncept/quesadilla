FROM docker.io/ubuntu:26.04

ARG GO_VERSION=1.27.3

RUN apt-get update
RUN apt-get -y upgrade
RUN apt-get -y install sudo wget curl vim git unzip bash

# Install Go
# split this up, something isnt' quite working !?!?
RUN GO_ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/') && \
    echo GO_VERSION=${GO_VERSION} GO_ARCH=${GO_ARCH}
RUN \
    GO_ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/') && \
    wget -q https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz -O /tmp/go.tar.gz

RUN rm -rf /usr/local/go && \
    tar -C /usr/local -xzf /tmp/go.tar.gz && \
    rm /tmp/go.tar.gz

ENV PATH="${PATH}:/usr/local/go/bin"

# # Fix ownership for ubuntu user
RUN chown -R ubuntu:ubuntu /usr/local/go

# User
RUN usermod -aG sudo ubuntu
RUN echo "ubuntu:ubuntu" | chpasswd

USER ubuntu
WORKDIR /home/ubuntu

# Install Opencode
RUN \
    curl -fsSL https://raw.githubusercontent.com/opencode-ai/opencode/refs/heads/main/install | bash

