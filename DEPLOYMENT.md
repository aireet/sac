# Deployment Guide

This guide provides step-by-step instructions for deploying the SAC platform to production.

## Prerequisites

- Kubernetes cluster (v1.28+)
- PostgreSQL database (v14+)
- Docker registry access
- kubectl configured
- Domain name with DNS configured

## Step 1: Database Setup

### 1.1 Database Connection

The database configuration is set in k8s/secrets/db-secret.yaml:
- Host: pgm-uf68x0dfyoth4u5g.pg.rds.aliyuncs.com
- Port: 1921
- User: sandbox
- Database: sandbox

**Note**: The database host appears to be an Alibaba Cloud RDS instance with internal network access only. If deploying to production:

1. Ensure your Kubernetes cluster can reach the database (VPC peering or VPN)
2. Or use a publicly accessible database for testing
3. Or deploy PostgreSQL within the cluster

### 1.2 Run Migrations

For local testing with accessible database:

```bash
cd backend

# Build migration tool
go build -o bin/migrate ./cmd/migrate

# Run migrations
export DB_HOST=your-db-host
export DB_PORT=5432
export DB_USER=sandbox
export DB_PASSWORD="4SOZfo6t6Oyj9A=="
export DB_NAME=sandbox

./bin/migrate -action=up

# Seed test data (creates admin user and 5 official skills)
./bin/migrate -action=seed

# Check migration status
./bin/migrate -action=status
```

For Kubernetes deployment, migrations will be run as a Job.

## Step 2: Build and Push Docker Images

### 2.1 Claude Code Container Image

```bash
cd docker/claude-code

# Update registry in the command
export REGISTRY=docker-register-registry-vpc.cn-shanghai.cr.aliyuncs.com/dev
export IMAGE_NAME=sac
export TAG=v1.0.0

# Build image
docker build -t ${REGISTRY}/${IMAGE_NAME}:${TAG} .

# Login to registry
docker login ${REGISTRY}

# Push image
docker push ${REGISTRY}/${IMAGE_NAME}:${TAG}

# Also tag as latest
docker tag ${REGISTRY}/${IMAGE_NAME}:${TAG} ${REGISTRY}/${IMAGE_NAME}:latest
docker push ${REGISTRY}/${IMAGE_NAME}:latest
```

### 2.2 Backend Services Images

You'll need to create Dockerfiles for the backend services:

**backend/cmd/api-gateway/Dockerfile**:
```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o api-gateway ./cmd/api-gateway

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/api-gateway .
EXPOSE 8080
CMD ["./api-gateway"]
```

**backend/cmd/ws-proxy/Dockerfile**:
```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o ws-proxy ./cmd/ws-proxy

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/ws-proxy .
EXPOSE 8081
CMD ["./ws-proxy"]
```

Build and push:

```bash
cd backend

# API Gateway
docker build -t ${REGISTRY}/sac-api-gateway:${TAG} -f cmd/api-gateway/Dockerfile .
docker push ${REGISTRY}/sac-api-gateway:${TAG}

# WebSocket Proxy
docker build -t ${REGISTRY}/sac-ws-proxy:${TAG} -f cmd/ws-proxy/Dockerfile .
docker push ${REGISTRY}/sac-ws-proxy:${TAG}
```

### 2.3 Frontend Image

**frontend/Dockerfile**:
```dockerfile
FROM node:18-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
```

**frontend/nginx.conf**:
```nginx
server {
    listen 80;
    server_name _;
    root /usr/share/nginx/html;
    index index.html;

    location / {
        try_files $uri $uri/ /index.html;
    }

    location /api/ {
        proxy_pass http://api-gateway:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }

    location /ws/ {
        proxy_pass http://ws-proxy:8081;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_read_timeout 86400;
    }
}
```

Build and push:

```bash
cd frontend

# Update API URLs in .env.production
cat > .env.production << EOF
VITE_API_URL=/api
VITE_WS_URL=/ws
EOF

docker build -t ${REGISTRY}/sac-frontend:${TAG} .
docker push ${REGISTRY}/sac-frontend:${TAG}
```

## Step 3: Kubernetes Deployment

### 3.1 Create Namespace

```bash
kubectl create namespace sac
kubectl config set-context --current --namespace=sac
```

### 3.2 Deploy Database Secret

```bash
kubectl apply -f k8s/secrets/db-secret.yaml
```

### 3.3 Deploy Services

Update image references in deployment files to match your registry:

```bash
# Update all image references
sed -i "s|docker-register-registry-vpc.cn-shanghai.cr.aliyuncs.com/dev|${REGISTRY}|g" k8s/deployments/*.yaml

# Apply deployments
kubectl apply -f k8s/deployments/api-gateway.yaml
kubectl apply -f k8s/deployments/ws-proxy.yaml
kubectl apply -f k8s/deployments/frontend.yaml

# Apply services
kubectl apply -f k8s/services/api-gateway-service.yaml
kubectl apply -f k8s/services/ws-proxy-service.yaml
kubectl apply -f k8s/services/frontend-service.yaml
```

### 3.4 Configure Istio (Optional)

If using Istio:

```bash
# Install Istio if not already installed
istioctl install --set profile=default

# Enable Istio injection
kubectl label namespace sac istio-injection=enabled

# Apply Istio gateway and virtual service
kubectl apply -f k8s/istio/gateway.yaml
kubectl apply -f k8s/istio/virtualservice.yaml
```

### 3.5 Configure Ingress (Alternative to Istio)

If not using Istio, create an Ingress:

**k8s/ingress.yaml**:
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: sac-ingress
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: /
    nginx.ingress.kubernetes.io/proxy-read-timeout: "3600"
    nginx.ingress.kubernetes.io/proxy-send-timeout: "3600"
spec:
  ingressClassName: nginx
  rules:
  - host: sac.your-domain.com
    http:
      paths:
      - path: /api
        pathType: Prefix
        backend:
          service:
            name: api-gateway
            port:
              number: 8080
      - path: /ws
        pathType: Prefix
        backend:
          service:
            name: ws-proxy
            port:
              number: 8081
      - path: /
        pathType: Prefix
        backend:
          service:
            name: frontend
            port:
              number: 80
```

```bash
kubectl apply -f k8s/ingress.yaml
```

## Step 4: Verify Deployment

### 4.1 Check Pod Status

```bash
# Check all pods are running
kubectl get pods

# Expected output:
# NAME                           READY   STATUS    RESTARTS   AGE
# api-gateway-xxxxxxxxxx-xxxxx   1/1     Running   0          2m
# ws-proxy-xxxxxxxxxx-xxxxx      1/1     Running   0          2m
# frontend-xxxxxxxxxx-xxxxx      1/1     Running   0          2m
```

### 4.2 Check Logs

```bash
# API Gateway logs
kubectl logs -l app=api-gateway --tail=50

# WebSocket Proxy logs
kubectl logs -l app=ws-proxy --tail=50

# Frontend logs
kubectl logs -l app=frontend --tail=50
```

### 4.3 Test API Endpoints

```bash
# Get API Gateway service IP
export API_URL=$(kubectl get svc api-gateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

# Test health endpoint
curl http://${API_URL}:8080/health

# Test skills API
curl http://${API_URL}:8080/api/skills
```

### 4.4 Test WebSocket Connection

```bash
# Port forward for local testing
kubectl port-forward svc/ws-proxy 8081:8081

# In another terminal, test WebSocket connection
# You can use a WebSocket client tool like wscat:
npm install -g wscat
wscat -c ws://localhost:8081/ws/1/test-session-123
```

## Step 5: DNS Configuration

Point your domain to the Ingress/Gateway IP:

```bash
# Get Ingress IP
kubectl get ingress sac-ingress

# Or if using Istio:
kubectl get svc istio-ingressgateway -n istio-system

# Create A record:
# sac.your-domain.com -> <INGRESS_IP>
```

## Step 6: SSL/TLS Configuration

### Using cert-manager (Recommended)

```bash
# Install cert-manager
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml

# Create ClusterIssuer for Let's Encrypt
cat <<EOF | kubectl apply -f -
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: your-email@example.com
    privateKeySecretRef:
      name: letsencrypt-prod
    solvers:
    - http01:
        ingress:
          class: nginx
EOF

# Update Ingress with TLS
kubectl annotate ingress sac-ingress cert-manager.io/cluster-issuer=letsencrypt-prod
kubectl patch ingress sac-ingress --type=json -p='[{"op": "add", "path": "/spec/tls", "value": [{"hosts": ["sac.your-domain.com"], "secretName": "sac-tls"}]}]'
```

## Step 7: Monitoring and Observability

### 7.1 Deploy Prometheus and Grafana

```bash
# Add Prometheus Helm repo
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

# Install Prometheus
helm install prometheus prometheus-community/kube-prometheus-stack -n monitoring --create-namespace

# Access Grafana
kubectl port-forward svc/prometheus-grafana 3000:80 -n monitoring
# Username: admin, Password: prom-operator
```

### 7.2 Application Metrics

Add metrics endpoints to your Go services using prometheus/client_golang.

## Step 8: Backup and Disaster Recovery

### 8.1 Database Backups

```bash
# Create CronJob for database backups
kubectl apply -f - <<EOF
apiVersion: batch/v1
kind: CronJob
metadata:
  name: db-backup
spec:
  schedule: "0 2 * * *"  # Daily at 2 AM
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: pg-dump
            image: postgres:14
            command:
            - /bin/bash
            - -c
            - |
              pg_dump -h \$DB_HOST -U \$DB_USER -d \$DB_NAME > /backup/backup-\$(date +%Y%m%d).sql
              # Upload to S3/OSS here
            env:
            - name: DB_HOST
              value: "pgm-uf68x0dfyoth4u5g.pg.rds.aliyuncs.com"
            - name: DB_USER
              valueFrom:
                secretKeyRef:
                  name: db-secret
                  key: username
            - name: PGPASSWORD
              valueFrom:
                secretKeyRef:
                  name: db-secret
                  key: password
            - name: DB_NAME
              value: "sandbox"
            volumeMounts:
            - name: backup
              mountPath: /backup
          volumes:
          - name: backup
            persistentVolumeClaim:
              claimName: db-backup-pvc
          restartPolicy: OnFailure
EOF
```

## Troubleshooting

### Database Connection Timeout

If you see "dial tcp ... i/o timeout":

1. **Check network connectivity**:
   ```bash
   kubectl run -it --rm debug --image=alpine --restart=Never -- sh
   apk add postgresql-client
   psql -h pgm-uf68x0dfyoth4u5g.pg.rds.aliyuncs.com -p 1921 -U sandbox -d sandbox
   ```

2. **Verify database whitelist**: Ensure Kubernetes cluster IPs are whitelisted in RDS

3. **Use port forwarding for testing**:
   ```bash
   # On a machine that can access the database
   ssh -L 5432:pgm-uf68x0dfyoth4u5g.pg.rds.aliyuncs.com:1921 user@bastion-host

   # Then run migrations locally
   export DB_HOST=localhost
   export DB_PORT=5432
   ./bin/migrate -action=up
   ```

### Pod Creation Fails

Check RBAC permissions:

```bash
# Ensure ws-proxy service account has permissions
kubectl get clusterrole ws-proxy-role
kubectl get clusterrolebinding ws-proxy-binding

# Check pod events
kubectl describe pod <pod-name>
```

### WebSocket Connection Drops

Check timeout settings:

```bash
# Increase timeouts in Istio VirtualService
kubectl edit virtualservice sac-routes
# Add timeout: 3600s

# Or in Ingress annotations
kubectl annotate ingress sac-ingress nginx.ingress.kubernetes.io/proxy-read-timeout="3600"
```

## Security Hardening

1. **Enable NetworkPolicies**: Restrict pod-to-pod communication
2. **Use PodSecurityPolicies**: Enforce security standards
3. **Rotate Secrets**: Implement secret rotation
4. **Enable Audit Logging**: Track all API access
5. **Implement Rate Limiting**: Prevent abuse
6. **Use RBAC**: Limit user permissions

## Scaling

### Horizontal Pod Autoscaling

```bash
kubectl autoscale deployment api-gateway --cpu-percent=70 --min=2 --max=10
kubectl autoscale deployment ws-proxy --cpu-percent=70 --min=2 --max=10
```

### Database Scaling

For production, consider:
- Read replicas for query load
- Connection pooling (PgBouncer)
- Database sharding for large scale

## Next Steps

1. Set up CI/CD pipeline
2. Implement authentication (OAuth2, SAML)
3. Add monitoring dashboards
4. Create runbooks for operations
5. Document disaster recovery procedures
6. Set up log aggregation
