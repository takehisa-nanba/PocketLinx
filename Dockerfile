# PocketLinx only supports 'alpine' or 'ubuntu' base images
FROM ubuntu

# Prevent interactive prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive

# Install Go and build tools
# Install Go manually (Ubuntu 22.04 only has 1.18)
RUN apt-get update && apt-get install -y git make bash wget && wget https://go.dev/dl/go1.23.0.linux-amd64.tar.gz && tar -C /usr/local -xzf go1.23.0.linux-amd64.tar.gz && rm go1.23.0.linux-amd64.tar.gz && ln -s /usr/local/go/bin/go /usr/bin/go && ln -s /usr/local/go/bin/gofmt /usr/bin/gofmt

ENV PATH=$PATH:/usr/local/go/bin

# Set workspace
WORKDIR /app

COPY go.mod ./
# COPY go.sum ./ # Optional as before

CMD ["bash"]
