package system

import (
	"context"
	"time"

	"github.com/robfig/cron/v3"
)

const scheduledSyncTimeout = 30 * time.Minute

type Scheduler struct {
	cron *cron.Cron
}

func NewScheduler(syncer *SyncService) *Scheduler {
	scheduler := &Scheduler{cron: cron.New()}
	jobs := map[string]string{
		"0 0 0/1 * * ?":  "sonarr-title",
		"0 15 0 * * ?":   "sonarr-rule",
		"0 30 0/1 * * ?": "radarr-title",
		"0 45 1 * * ?":   "radarr-rule",
	}
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	scheduler.cron = cron.New(cron.WithParser(parser))
	for spec, job := range jobs {
		jobName := job
		_, _ = scheduler.cron.AddFunc(spec, func() {
			ctx, cancel := context.WithTimeout(context.Background(), scheduledSyncTimeout)
			defer cancel()
			_ = syncer.Run(ctx, jobName)
		})
	}
	return scheduler
}

func (s *Scheduler) Start() {
	s.cron.Start()
}

func (s *Scheduler) Stop() {
	if s == nil || s.cron == nil {
		return
	}
	ctx := s.cron.Stop()
	<-ctx.Done()
}
