package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/echotech/sac/internal/database"
	"github.com/echotech/sac/internal/models"
	"github.com/echotech/sac/migrations"
	"github.com/echotech/sac/pkg/config"
	"github.com/uptrace/bun/migrate"
)

func main() {
	var action string
	flag.StringVar(&action, "action", "up", "Migration action: up, down, status, seed")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize database
	if err := database.Initialize(cfg); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	ctx := context.Background()
	migrator := migrate.NewMigrator(database.DB, migrations.Migrations)

	// Initialize migration tables if needed
	if err := migrator.Init(ctx); err != nil {
		log.Fatalf("Failed to initialize migrator: %v", err)
	}

	switch action {
	case "up":
		if err := migrator.Lock(ctx); err != nil {
			log.Fatalf("Failed to lock migrations: %v", err)
		}
		defer migrator.Unlock(ctx)

		group, err := migrator.Migrate(ctx)
		if err != nil {
			log.Fatalf("Failed to migrate: %v", err)
		}
		if group.IsZero() {
			log.Println("No new migrations to run")
		} else {
			log.Printf("Migrated to %s\n", group)
		}

	case "down":
		if err := migrator.Lock(ctx); err != nil {
			log.Fatalf("Failed to lock migrations: %v", err)
		}
		defer migrator.Unlock(ctx)

		group, err := migrator.Rollback(ctx)
		if err != nil {
			log.Fatalf("Failed to rollback: %v", err)
		}
		if group.IsZero() {
			log.Println("No migrations to rollback")
		} else {
			log.Printf("Rolled back %s\n", group)
		}

	case "status":
		ms, err := migrator.MigrationsWithStatus(ctx)
		if err != nil {
			log.Fatalf("Failed to get migration status: %v", err)
		}
		fmt.Printf("Migrations: %s\n", ms)

	case "seed":
		seedData(ctx)

	default:
		log.Fatalf("Unknown action: %s", action)
	}
}

func seedData(ctx context.Context) {
	log.Println("Seeding database...")

	// Create test user
	user := &models.User{
		Username:    "admin",
		Email:       "admin@example.com",
		DisplayName: "Admin User",
	}

	_, err := database.DB.NewInsert().
		Model(user).
		On("CONFLICT (username) DO NOTHING").
		Exec(ctx)
	if err != nil {
		log.Fatalf("Failed to create user: %v", err)
	}
	log.Println("Created test user")

	// Get user ID
	err = database.DB.NewSelect().
		Model(user).
		Where("username = ?", "admin").
		Scan(ctx)
	if err != nil {
		log.Fatalf("Failed to get user: %v", err)
	}

	// Create official skills
	skills := []models.Skill{
		{
			Name:        "本周销售额查询",
			Description: "查询本周的销售额统计数据",
			Icon:        "💰",
			Category:    "数据查询",
			Prompt:      "请帮我查询本周的销售额，包括总金额、订单数量和平均客单价。",
			IsOfficial:  true,
			CreatedBy:   user.ID,
			IsPublic:    true,
		},
		{
			Name:        "用户增长趋势分析",
			Description: "分析最近30天的用户增长趋势",
			Icon:        "📈",
			Category:    "数据分析",
			Prompt:      "请帮我分析最近30天的用户增长趋势，包括日新增用户数、累计用户数和增长率。",
			IsOfficial:  true,
			CreatedBy:   user.ID,
			IsPublic:    true,
		},
		{
			Name:        "订单统计报表",
			Description: "生成订单统计报表",
			Icon:        "📦",
			Category:    "报表生成",
			Prompt:      "请帮我生成订单统计报表，包括订单总数、已完成订单、待处理订单和已取消订单。",
			IsOfficial:  true,
			CreatedBy:   user.ID,
			IsPublic:    true,
		},
		{
			Name:        "渠道转化率分析",
			Description: "分析各渠道的转化率",
			Icon:        "🎯",
			Category:    "数据分析",
			Prompt:      "请帮我分析各个渠道的转化率，包括访问量、注册量、付费量和转化率。",
			IsOfficial:  true,
			CreatedBy:   user.ID,
			IsPublic:    true,
		},
		{
			Name:        "自定义时间段查询",
			Description: "查询指定时间段的数据",
			Icon:        "📅",
			Category:    "数据查询",
			Prompt:      "请帮我查询 {{startDate}} 到 {{endDate}} 之间的数据。要求：\n1. 统计总交易额\n2. 统计订单数量\n3. 按天展示趋势图",
			Parameters: models.SkillParameters{
				{
					Name:     "startDate",
					Label:    "开始日期",
					Type:     "date",
					Required: true,
				},
				{
					Name:     "endDate",
					Label:    "结束日期",
					Type:     "date",
					Required: true,
				},
			},
			IsOfficial: true,
			CreatedBy:  user.ID,
			IsPublic:   true,
		},
	}

	for _, skill := range skills {
		_, err := database.DB.NewInsert().
			Model(&skill).
			On("CONFLICT (name) DO NOTHING").
			Exec(ctx)
		if err != nil {
			log.Printf("Failed to create skill %s: %v", skill.Name, err)
		}
	}

	log.Println("Seeded official skills")
	log.Println("Database seeding completed!")
}
