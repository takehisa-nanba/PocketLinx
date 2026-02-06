# PocketLinx only supports 'alpine' or 'ubuntu' base images
FROM ubuntu

# Prevent interactive prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive

# Install Go and build tools
# Install Go manually (from local file to avoid network issues)
COPY go1.23.0.linux-amd64.tar.gz /tmp/
RUN apt-get update && apt-get install -y git make bash && tar -C /usr/local -xzf /tmp/go1.23.0.linux-amd64.tar.gz && rm /tmp/go1.23.0.linux-amd64.tar.gz && ln -s /usr/local/go/bin/go /usr/bin/go && ln -s /usr/local/go/bin/gofmt /usr/bin/gofmt

ENV PATH=$PATH:/usr/local/go/bin

# Set workspace
WORKDIR /app

COPY go.mod ./
# COPY go.sum ./ # Optional as before

# Add plx wrapper for dogfooding
RUN echo '#!/bin/sh' > /usr/local/bin/plx && \
    echo 'exec go run /app/cmd/plx/main.go "$@"' >> /usr/local/bin/plx && \
    chmod +x /usr/local/bin/plx

CMD ["bash"]
