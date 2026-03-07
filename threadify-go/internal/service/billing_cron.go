package service

import (
	"context"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/repository/postgres"
	"go.uber.org/zap"
)

type BillingCron struct {
	planRepo       *postgres.PlanRepository
	billingRepo    *postgres.BillingRepository
	billingService *BillingService
	valkeyClient   interfaces.ValkeyClient
	logger         *zap.Logger
	cron           *cron.Cron
}

func NewBillingCron(
	planRepo *postgres.PlanRepository,
	billingRepo *postgres.BillingRepository,
	billingService *BillingService,
	valkeyClient interfaces.ValkeyClient,
	logger *zap.Logger,
) *BillingCron {
	return &BillingCron{
		planRepo:       planRepo,
		billingRepo:    billingRepo,
		billingService: billingService,
		valkeyClient:   valkeyClient,
		logger:         logger,
		cron:           cron.New(cron.WithLocation(time.UTC)),
	}
}

func (c *BillingCron) Start() error {
	// Schedule to run once a day at midnight UTC
	_, err := c.cron.AddFunc("0 0 * * *", func() {
		ctx := context.Background()
		c.runWithLock(ctx)
	})
	if err != nil {
		c.logger.Error("failed to schedule billing cron", zap.Error(err))
		return err
	}

	c.cron.Start()
	c.logger.Info("billing cron started — scheduled to run daily at 00:00 UTC")
	return nil
}

func (c *BillingCron) runWithLock(ctx context.Context) {
	const lockKey = "cron:billing:lock"
	const lockTTL = 23 * time.Hour // Lock for almost the whole day

	// Try to acquire lock
	success, err := c.valkeyClient.SetNX(ctx, lockKey, "locked", lockTTL)
	if err != nil {
		c.logger.Error("billing cron: failed to check distributed lock", zap.Error(err))
		return
	}
	if !success {
		// Another instance is already running/has run it today
		return
	}

	c.logger.Info("billing cron: acquired leader lock, starting daily run")
	c.processAll(ctx)
}

func (c *BillingCron) Stop() error {
	if c.cron != nil {
		c.cron.Stop()
	}
	return nil
}

type billingJob struct {
	due         bool
	periodStart time.Time
	periodEnd   time.Time
	isCycleEnd  bool
}

func (c *BillingCron) processAll(ctx context.Context) {
	plans, err := c.planRepo.ListActivePlans(ctx)
	if err != nil {
		c.logger.Error("billing cron: failed to list active plans", zap.Error(err))
		return
	}

	now := time.Now().UTC()
	var processed, skipped int

	for _, plan := range plans {
		job := c.isDue(ctx, &plan, now)
		if !job.due {
			skipped++
			continue
		}

		if err := c.billingService.SnapshotAndBill(ctx, plan.CompanyID, job.periodStart, job.periodEnd, job.isCycleEnd); err != nil {
			c.logger.Error("billing cron: snapshot failed",
				zap.String("company_id", plan.CompanyID),
				zap.Error(err),
			)
			continue
		}

		if job.isCycleEnd {
			newStart := plan.BillingEnd
			var newEnd time.Time
			if plan.BillingCycle == models.BillingCycleMonthly {
				newEnd = newStart.AddDate(0, 1, 0)
			} else {
				newEnd = newStart.AddDate(1, 0, 0)
			}
			if err := c.planRepo.UpdatePlanTier(ctx, plan.CompanyID, plan.SubscriptionTier, plan.BillingCycle, newStart, newEnd); err != nil {
				c.logger.Error("billing cron: failed to advance billing period",
					zap.String("company_id", plan.CompanyID),
					zap.Error(err),
				)
			}
		}

		processed++
	}

	if processed > 0 || skipped > 0 {
		c.logger.Info("billing cron: daily run complete",
			zap.Int("processed", processed),
			zap.Int("skipped", skipped),
		)
	}
}

func (c *BillingCron) isDue(ctx context.Context, plan *models.CompanyPlan, now time.Time) billingJob {
	if now.After(plan.BillingEnd) || now.Equal(plan.BillingEnd) {
		return billingJob{
			due:         true,
			periodStart: plan.BillingStart,
			periodEnd:   plan.BillingEnd,
			isCycleEnd:  true,
		}
	}

	if plan.BillingCycle == models.BillingCycleYearly {
		lastSnapshot, err := c.billingRepo.FindLatestSnapshot(ctx, plan.CompanyID)
		if err != nil {
			c.logger.Warn("billing cron: failed to find latest snapshot, skipping",
				zap.String("company_id", plan.CompanyID),
				zap.Error(err),
			)
			return billingJob{due: false}
		}

		var lastPeriodEnd time.Time
		if lastSnapshot != nil {
			lastPeriodEnd = lastSnapshot.PeriodEnd
		} else {
			lastPeriodEnd = plan.BillingStart
		}

		nextDue := lastPeriodEnd.AddDate(0, 1, 0)
		if now.After(nextDue) || now.Equal(nextDue) {
			return billingJob{
				due:         true,
				periodStart: lastPeriodEnd,
				periodEnd:   nextDue,
				isCycleEnd:  false,
			}
		}
	}

	return billingJob{due: false}
}
