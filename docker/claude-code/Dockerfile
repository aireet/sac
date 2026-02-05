FROM ubuntu:22.04

# Set environment variables
ENV DEBIAN_FRONTEND=noninteractive
ENV USER_ID=""
ENV SESSION_ID=""

# Install basic tools
RUN apt-get update && apt-get install -y \
    curl \
    wget \
    git \
    vim \
    bash \
    ca-certificates \
    build-essential \
    python3 \
    python3-pip \
    postgresql-client \
    mysql-client \
    redis-tools \
    && rm -rf /var/lib/apt/lists/*

# Install ttyd
RUN wget https://github.com/tsl0922/ttyd/releases/download/1.7.4/ttyd.x86_64 -O /usr/local/bin/ttyd \
    && chmod +x /usr/local/bin/ttyd

# Install Claude Code CLI
# Note: Replace with actual Claude Code installation steps
RUN curl -fsSL https://claude.ai/install.sh | bash || echo "Claude Code installation placeholder"

# Create workspace directory
RUN mkdir -p /workspace
WORKDIR /workspace

# Copy entrypoint script
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Expose ttyd port
EXPOSE 7681

# Set entrypoint
ENTRYPOINT ["/entrypoint.sh"]
