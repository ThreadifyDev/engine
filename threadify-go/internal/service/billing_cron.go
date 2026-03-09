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
	lockKey := "cron:billing:lock:" + time.Now().UTC().Format("2006-01-02")
	const lockTTL = 30 * time.Minute

	success, err := c.valkeyClient.SetNX(ctx, lockKey, "locked", lockTTL)
	if err != nil {
		c.logger.Error("billing cron: failed to check distributed lock", zap.Error(err))
		return
	}
	if !success {
		return
	}

	done := make(chan struct{})
	defer close(done)

	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := c.valkeyClient.Expire(context.Background(), lockKey, lockTTL); err != nil {
					c.logger.Error("billing cron: failed to renew lock heartbeat", zap.Error(err))
				}
			case <-done:
				return
			}
		}
	}()

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
	reason      models.SnapshotReason
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

		if err := c.billingService.SnapshotAndBill(ctx, plan.CompanyID, job.periodStart, job.periodEnd, job.reason); err != nil {
			c.logger.Error("billing cron: snapshot failed",
				zap.String("company_id", plan.CompanyID),
				zap.Error(err),
			)
			continue
		}

		processed++
	}

	c.logger.Info("billing cron: daily run complete",
		zap.Int("processed", processed),
		zap.Int("skipped", skipped),
		zap.Int("total", len(plans)),
	)
}

func (c *BillingCron) isDue(ctx context.Context, plan *models.CompanyPlan, now time.Time) billingJob {
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

	gracePeriod := 15 * time.Minute
	safeToBillThreshold := nextDue.Add(gracePeriod)

	if now.Before(safeToBillThreshold) {
		if plan.BillingCycle == models.BillingCycleYearly && now.After(plan.BillingEnd.Add(gracePeriod)) {
			return billingJob{
				due:         true,
				periodStart: lastPeriodEnd,
				periodEnd:   plan.BillingEnd,
				reason:      models.SnapshotReasonYearlyRenewal,
			}
		}
		return billingJob{due: false}
	}

	switch plan.BillingCycle {
	case models.BillingCycleMonthly:
		return billingJob{
			due:         true,
			periodStart: lastPeriodEnd,
			periodEnd:   nextDue,
			reason:      models.SnapshotReasonMonthlyRenewal,
		}

	case models.BillingCycleYearly:
		isYearEnd := now.After(plan.BillingEnd.Add(gracePeriod))
		reason := models.SnapshotReasonOverageOnly
		if isYearEnd {
			reason = models.SnapshotReasonYearlyRenewal
		}
		return billingJob{
			due:         true,
			periodStart: lastPeriodEnd,
			periodEnd:   nextDue,
			reason:      reason,
		}
	}

	return billingJob{due: false}
}
