FROM swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/ubuntu:22.04

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
    jq \
    && rm -rf /var/lib/apt/lists/*

# Install Node.js 22 LTS
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y nodejs \
    && rm -rf /var/lib/apt/lists/*

# Install Claude Code CLI via npm
RUN npm config set registry https://registry.npmmirror.com && npm install -g @anthropic-ai/claude-code

# Install ttyd
RUN wget https://github.com/tsl0922/ttyd/releases/download/1.7.4/ttyd.x86_64 -O /usr/local/bin/ttyd \
    && chmod +x /usr/local/bin/ttyd

# Create workspace directory
RUN mkdir -p /workspace
WORKDIR /workspace

# Copy hooks
COPY hooks/ /hooks/
RUN chmod +x /hooks/*.sh

# Copy entrypoint script
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Expose ttyd port
EXPOSE 7681

# Set entrypoint
ENTRYPOINT ["/entrypoint.sh"]
