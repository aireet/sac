FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive
ENV USER_ID=""
ENV SESSION_ID=""

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
    dtach \
    locales \
    && locale-gen en_US.UTF-8 \
    && rm -rf /var/lib/apt/lists/*

ENV LANG=en_US.UTF-8
ENV LC_ALL=en_US.UTF-8

RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y nodejs \
    && rm -rf /var/lib/apt/lists/*

RUN npm install -g @anthropic-ai/claude-code

RUN wget https://github.com/tsl0922/ttyd/releases/download/1.7.4/ttyd.x86_64 -O /usr/local/bin/ttyd \
    && chmod +x /usr/local/bin/ttyd

RUN pip3 install --no-cache-dir plotly pandas numpy

RUN mkdir -p /workspace
WORKDIR /workspace

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 7681

ENTRYPOINT ["/entrypoint.sh"]
