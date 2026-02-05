# PocketLinx only supports 'alpine' or 'ubuntu' base images
FROM alpine

# Install Go and build tools
# apk repositories typically have recent Go versions (e.g. 1.22/1.23 in Alpine 3.20+)
RUN apk add --no-cache go git make bash musl-dev util-linux

# Set workspace
WORKDIR /app

COPY go.mod ./
# COPY go.sum ./ # Optional as before

# PocketLinx defaults to root, which is fine for this dev container
CMD ["bash"]
