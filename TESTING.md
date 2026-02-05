# Testing Guide

This document provides comprehensive testing instructions for the SAC platform.

## Testing Overview

The SAC platform consists of several components that need to be tested:
1. Backend Services (API Gateway, WebSocket Proxy)
2. Frontend Application (Vue 3 + xterm.js)
3. Database Layer (PostgreSQL + bun ORM)
4. Kubernetes Integration (Pod lifecycle management)
5. End-to-End User Workflows

## Prerequisites

- Go 1.21+
- Node.js 18+
- PostgreSQL 14+
- Docker
- kubectl (for K8s tests)

## Unit Testing

### Backend Unit Tests

Create test files for each package:

**backend/internal/models/skill_test.go**:
```go
package models

import (
	"encoding/json"
	"testing"
)

func TestSkillParametersScan(t *testing.T) {
	params := SkillParameters{
		{Name: "startDate", Label: "Start Date", Type: "date", Required: true},
		{Name: "endDate", Label: "End Date", Type: "date", Required: true},
	}

	jsonData, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var scanned SkillParameters
	err = scanned.Scan(jsonData)
	if err != nil {
		t.Fatalf("Failed to scan: %v", err)
	}

	if len(scanned) != 2 {
		t.Errorf("Expected 2 parameters, got %d", len(scanned))
	}
}
```

Run backend tests:
```bash
cd backend
go test ./...
```

### Frontend Unit Tests

**frontend/src/components/SkillPanel/SkillPanel.test.ts**:
```typescript
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import SkillPanel from './SkillPanel.vue'

describe('SkillPanel', () => {
  it('renders skills correctly', () => {
    const wrapper = mount(SkillPanel)
    expect(wrapper.find('.skill-panel').exists()).toBe(true)
  })

  it('executes skill with parameters', async () => {
    const wrapper = mount(SkillPanel)
    // Test parameter substitution
    const skill = {
      prompt: 'Query from {{startDate}} to {{endDate}}',
      parameters: [
        { name: 'startDate', required: true },
        { name: 'endDate', required: true }
      ]
    }

    // Simulate parameter input
    // Assert command is correctly formatted
  })
})
```

Run frontend tests:
```bash
cd frontend
npm run test
```

## Integration Testing

### Database Integration Tests

Test database connectivity and CRUD operations:

**backend/internal/database/db_test.go**:
```go
package database

import (
	"context"
	"testing"

	"github.com/echotech/sac/internal/models"
	"github.com/echotech/sac/pkg/config"
)

func TestDatabaseConnection(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Skipf("Skipping database test: %v", err)
	}

	err = Initialize(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer Close()

	// Test ping
	err = DB.Ping()
	if err != nil {
		t.Fatalf("Failed to ping database: %v", err)
	}
}

func TestSkillCRUD(t *testing.T) {
	// Test create, read, update, delete operations
	ctx := context.Background()

	skill := &models.Skill{
		Name:        "Test Skill",
		Description: "Test Description",
		Prompt:      "Test prompt",
		CreatedBy:   1,
	}

	// Create
	_, err := DB.NewInsert().Model(skill).Exec(ctx)
	if err != nil {
		t.Fatalf("Failed to create skill: %v", err)
	}

	// Read
	var retrieved models.Skill
	err = DB.NewSelect().Model(&retrieved).Where("id = ?", skill.ID).Scan(ctx)
	if err != nil {
		t.Fatalf("Failed to retrieve skill: %v", err)
	}

	if retrieved.Name != "Test Skill" {
		t.Errorf("Expected 'Test Skill', got '%s'", retrieved.Name)
	}

	// Clean up
	_, err = DB.NewDelete().Model(skill).WherePK().Exec(ctx)
	if err != nil {
		t.Fatalf("Failed to delete skill: %v", err)
	}
}
```

Run with database connection:
```bash
export DB_HOST=localhost
export DB_PORT=5432
export DB_USER=sandbox
export DB_PASSWORD=password
export DB_NAME=sandbox_test

go test ./internal/database -v
```

### API Integration Tests

Test API endpoints:

**backend/cmd/api-gateway/main_test.go**:
```go
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSkillsAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := setupRouter()

	t.Run("GET /api/skills", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/skills", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.Code)
		}
	})

	t.Run("POST /api/skills", func(t *testing.T) {
		skill := map[string]interface{}{
			"name":        "Test Skill",
			"description": "Test",
			"prompt":      "Test prompt",
		}

		body, _ := json.Marshal(skill)
		req, _ := http.NewRequest("POST", "/api/skills", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.Code)
		}
	})
}
```

## Component Testing

### Terminal Component Test

Test xterm.js terminal integration:

1. Start backend services locally:
```bash
# Terminal 1: Start WebSocket Proxy
cd backend
go run ./cmd/ws-proxy

# Terminal 2: Start API Gateway
go run ./cmd/api-gateway
```

2. Start frontend:
```bash
cd frontend
npm run dev
```

3. Manual test checklist:
- [ ] Terminal loads and displays prompt
- [ ] Can type characters and see them echoed
- [ ] Enter key sends commands
- [ ] Terminal output is displayed correctly
- [ ] Terminal resizes with window
- [ ] WebSocket reconnects after disconnect
- [ ] Copy/paste works correctly

### Skill Panel Test

1. Load test data:
```bash
cd backend
./bin/migrate -action=seed
```

2. Verify skills display:
- [ ] Official skills are displayed
- [ ] Skills are grouped by category
- [ ] Search functionality works
- [ ] Click skill button sends command to terminal

3. Test parameterized skill:
- [ ] Click skill with parameters
- [ ] Parameter modal appears
- [ ] Required fields are validated
- [ ] Parameters are correctly substituted in prompt
- [ ] Command is sent to terminal after submission

### Skill Register Test

1. Create new skill:
- [ ] Fill in skill form (name, description, icon, category, prompt)
- [ ] Add parameters with different types (text, date, number, select)
- [ ] Mark as public
- [ ] Submit and verify it appears in skill list

2. Edit existing skill:
- [ ] Click edit on user-created skill
- [ ] Modify fields
- [ ] Save and verify changes

3. Delete skill:
- [ ] Click delete on user-created skill
- [ ] Confirm deletion
- [ ] Verify skill is removed

4. Fork public skill:
- [ ] Find public skill from another user
- [ ] Click fork button
- [ ] Modify and save
- [ ] Verify forked skill is in user's list

## End-to-End Testing

### Scenario 1: New User Workflow

```bash
# 1. User opens application
# Expected: Frontend loads, login prompt appears (mock auth redirects to main view)

# 2. User sees terminal and skill panel
# Expected: Terminal connects via WebSocket, displays prompt

# 3. User clicks "本周销售额查询" skill
# Expected: Command is sent to terminal, Claude responds with query results

# 4. User types custom query
# Expected: Query is executed, results displayed

# 5. User closes browser
# Expected: WebSocket closes gracefully, session remains active

# 6. User reopens browser
# Expected: Reconnects to same session, history is preserved
```

### Scenario 2: Skill Creation Workflow

```bash
# 1. User navigates to "Manage" tab
# Expected: Skill editor loads, shows existing skills

# 2. User clicks "Create New Skill"
# Expected: Blank form appears

# 3. User fills in skill details:
#    - Name: "Custom Sales Report"
#    - Category: "Data Query"
#    - Prompt: "Generate sales report for {{product}} from {{startDate}} to {{endDate}}"
#    - Parameters: product (select), startDate (date), endDate (date)
# Expected: Form validates input

# 4. User clicks "Save"
# Expected: Skill is saved to database, appears in skill list

# 5. User switches to "Skills" tab
# Expected: New skill appears in Data Query category

# 6. User clicks new skill
# Expected: Parameter modal appears with 3 inputs

# 7. User fills parameters and submits
# Expected: Prompt with substituted values is sent to terminal
```

### Scenario 3: Pod Lifecycle

```bash
# 1. User connects for first time
# Expected: ws-proxy creates new pod, waits for ready status

# 2. Check pod status
kubectl get pods -l user-id=1
# Expected: One pod running

# 3. User interacts with terminal
# Expected: Commands execute in pod, outputs returned

# 4. User disconnects
# Expected: Pod remains running, session marked as inactive

# 5. Wait 2 hours (or trigger timeout manually)
# Expected: Pod is stopped (implementation pending)

# 6. User reconnects within 7 days
# Expected: Pod is restarted, workspace data intact

# 7. Wait 7 days
# Expected: Pod is deleted, workspace PVC removed
```

## Load Testing

### WebSocket Connection Load Test

Use a tool like `artillery` to test WebSocket connections:

**loadtest/websocket-load.yml**:
```yaml
config:
  target: "ws://localhost:8081"
  phases:
    - duration: 60
      arrivalRate: 10
      name: "Warm up"
    - duration: 300
      arrivalRate: 50
      name: "Sustained load"
  websocket:
    timeout: 60000

scenarios:
  - name: "Connect and send commands"
    engine: "ws"
    flow:
      - connect:
          url: "/ws/{{ $randomNumber(1, 100) }}/test-{{ $randomNumber(1, 1000) }}"
      - think: 2
      - send: "echo 'Hello World'\r"
      - think: 1
      - send: "ls -la\r"
      - think: 5
```

Run load test:
```bash
npm install -g artillery
artillery run loadtest/websocket-load.yml
```

### API Load Test

**loadtest/api-load.yml**:
```yaml
config:
  target: "http://localhost:8080"
  phases:
    - duration: 60
      arrivalRate: 20
  http:
    timeout: 30

scenarios:
  - name: "Get skills"
    flow:
      - get:
          url: "/api/skills"

  - name: "Create skill"
    flow:
      - post:
          url: "/api/skills"
          json:
            name: "Test Skill {{ $randomNumber(1, 10000) }}"
            description: "Load test skill"
            prompt: "Test prompt"
            category: "Test"
```

## Performance Testing

### Database Query Performance

Test query performance with large datasets:

```sql
-- Insert test data
INSERT INTO skills (name, description, icon, category, prompt, is_official, created_by, is_public)
SELECT
  'Test Skill ' || generate_series,
  'Description ' || generate_series,
  '🧪',
  'Test',
  'Prompt ' || generate_series,
  false,
  1,
  true
FROM generate_series(1, 10000);

-- Test query performance
EXPLAIN ANALYZE SELECT * FROM skills WHERE category = 'Test' AND is_public = true;

-- Test pagination
EXPLAIN ANALYZE SELECT * FROM skills ORDER BY created_at DESC LIMIT 20 OFFSET 0;
```

### Frontend Bundle Size

Check bundle size:
```bash
cd frontend
npm run build

# Analyze bundle
npx vite-bundle-visualizer
```

Optimize if bundle is too large:
- Code splitting
- Lazy loading components
- Tree shaking
- Minification

## Security Testing

### SQL Injection Test

Test that bun ORM prevents SQL injection:

```go
// Should be safe due to parameterized queries
userInput := "'; DROP TABLE skills; --"
skill := &models.Skill{Name: userInput}
_, err := DB.NewInsert().Model(skill).Exec(ctx)
// Verify table still exists and data is safely escaped
```

### XSS Test

Test that frontend sanitizes user input:

```typescript
const maliciousSkill = {
  name: '<script>alert("XSS")</script>',
  description: '<img src=x onerror=alert("XSS")>',
  prompt: 'Normal prompt'
}

// Submit skill and verify it's displayed safely without executing
```

### WebSocket Security

Test WebSocket authentication and authorization:

```bash
# Try to connect without valid session
wscat -c ws://localhost:8081/ws/999/invalid-session

# Try to access another user's pod
wscat -c ws://localhost:8081/ws/1/someone-else-session

# Try to send malicious payloads
wscat -c ws://localhost:8081/ws/1/test-session
> \x00\x00\x00\x00  # null bytes
> $(rm -rf /)       # command injection attempt
```

## Automated Testing Pipeline

Create CI/CD pipeline with GitHub Actions:

**.github/workflows/test.yml**:
```yaml
name: Test Suite

on:
  push:
    branches: [main, develop]
  pull_request:
    branches: [main]

jobs:
  backend-tests:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:14
        env:
          POSTGRES_PASSWORD: postgres
          POSTGRES_DB: sandbox_test
        ports:
          - 5432:5432
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5

    steps:
      - uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Run tests
        env:
          DB_HOST: localhost
          DB_PORT: 5432
          DB_USER: postgres
          DB_PASSWORD: postgres
          DB_NAME: sandbox_test
        run: |
          cd backend
          go test -v ./...

  frontend-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Set up Node
        uses: actions/setup-node@v3
        with:
          node-version: '18'

      - name: Install dependencies
        run: |
          cd frontend
          npm ci

      - name: Run tests
        run: |
          cd frontend
          npm run test

      - name: Build
        run: |
          cd frontend
          npm run build

  e2e-tests:
    runs-on: ubuntu-latest
    needs: [backend-tests, frontend-tests]
    steps:
      - uses: actions/checkout@v3

      - name: Install Playwright
        run: npx playwright install --with-deps

      - name: Run E2E tests
        run: npx playwright test
```

## Test Coverage

### Backend Coverage

```bash
cd backend
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

# View coverage report
open coverage.html
```

Target: 80%+ coverage for critical paths:
- Database operations
- WebSocket proxy logic
- Skill CRUD operations
- Container management

### Frontend Coverage

```bash
cd frontend
npm run test:coverage

# View coverage report
open coverage/index.html
```

Target: 70%+ coverage for components:
- Terminal component
- Skill Panel
- Skill Register

## Test Checklist

Before deploying to production:

- [ ] All unit tests pass
- [ ] Integration tests pass
- [ ] E2E tests pass for critical workflows
- [ ] Load testing shows acceptable performance
- [ ] No security vulnerabilities found
- [ ] Database migrations run successfully
- [ ] Frontend builds without errors
- [ ] Docker images build successfully
- [ ] Kubernetes manifests are valid
- [ ] Monitoring and logging are working
- [ ] Backup and restore procedures tested
- [ ] Documentation is up to date

## Reporting Issues

When reporting a bug, include:
1. Steps to reproduce
2. Expected behavior
3. Actual behavior
4. Environment (browser, OS, etc.)
5. Logs (backend, frontend console, network tab)
6. Screenshots or video if applicable

## Continuous Testing

Implement continuous testing practices:
- Run tests on every commit
- Run integration tests nightly
- Run load tests weekly
- Review test coverage monthly
- Update tests when adding features
- Fix flaky tests immediately
