# Claude Code Container Image

This Docker image provides a containerized environment for Claude Code with ttyd WebSocket terminal access.

## Build

```bash
docker build -t docker-register-registry-vpc.cn-shanghai.cr.aliyuncs.com/dev/sac:latest .
```

## Push to Registry

```bash
docker push docker-register-registry-vpc.cn-shanghai.cr.aliyuncs.com/dev/sac:latest
```

## Local Test

```bash
docker run -p 7681:7681 \
  -e USER_ID=test-user \
  -e SESSION_ID=test-session \
  docker-register-registry-vpc.cn-shanghai.cr.aliyuncs.com/dev/sac:latest
```

Then open browser: http://localhost:7681

## Components

- **Ubuntu 22.04**: Base OS
- **ttyd**: WebSocket terminal server
- **Claude Code CLI**: AI coding assistant
- **Database Clients**: PostgreSQL, MySQL, Redis
- **Development Tools**: git, vim, curl, wget, python3

## Environment Variables

- `USER_ID`: User identifier
- `SESSION_ID`: Session identifier

## Ports

- `7681`: ttyd WebSocket server

## Volume Mounts

- `/workspace`: User's working directory
