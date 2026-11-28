package notifications

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"food-service/internal/maintenance"
	"food-service/internal/orders"
)

const (
	sqliteTimeLayout = "2006-01-02 15:04:05"
	outboxBatchSize  = 20
	outboxInterval   = 30 * time.Second
	maxRetryDelay    = 6 * time.Hour
)

type Config struct {
	BotToken       string
	AllowedUserIDs []int64
	PublicBaseURL  string
}

// LoadConfig reads the Telegram settings from the environment. An empty or
// incomplete configuration is reported as an error so main can log a clear
// warning and keep the rest of the service running without notifications.
func LoadConfig() (Config, error) {
	config := Config{
		BotToken:      strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		PublicBaseURL: strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")),
	}

	rawIDs := strings.TrimSpace(os.Getenv("TELEGRAM_ALLOWED_USER_IDS"))
	seen := make(map[int64]struct{})

	if rawIDs != "" {
		for _, entry := range strings.Split(rawIDs, ",") {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}

			userID, err := strconv.ParseInt(entry, 10, 64)
			if err != nil || userID <= 0 {
				return Config{}, fmt.Errorf("invalid TELEGRAM_ALLOWED_USER_IDS entry %q", entry)
			}

			if _, exists := seen[userID]; exists {
				continue
			}
			seen[userID] = struct{}{}
			config.AllowedUserIDs = append(config.AllowedUserIDs, userID)
		}
	}

	if config.BotToken == "" && len(config.AllowedUserIDs) == 0 && config.PublicBaseURL == "" {
		return Config{}, errors.New("TELEGRAM_BOT_TOKEN, TELEGRAM_ALLOWED_USER_IDS and PUBLIC_BASE_URL are not set")
	}
	if config.BotToken == "" {
		return Config{}, errors.New("TELEGRAM_BOT_TOKEN is not set")
	}
	if len(config.AllowedUserIDs) == 0 {
		return Config{}, errors.New("TELEGRAM_ALLOWED_USER_IDS is empty or invalid")
	}
	if config.PublicBaseURL == "" {
		return Config{}, errors.New("PUBLIC_BASE_URL is not set")
	}

	parsed, err := url.Parse(config.PublicBaseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return Config{}, errors.New("PUBLIC_BASE_URL must be an absolute http(s) URL")
	}
	config.PublicBaseURL = strings.TrimRight(config.PublicBaseURL, "/")

	return config, nil
}

// Notifier implements orders.Notifications. It writes outbox rows inside the
// order transaction and delivers Telegram messages in a single background
// goroutine.
type Notifier struct {
	db            *sql.DB
	orders        *orders.Service
	client        *Client
	recipients    []int64
	publicBaseURL string
	logger        *log.Logger
	wake          chan struct{}
	interval      time.Duration
	now           func() time.Time
	gate          *maintenance.Gate
}

// SetGate lets the notifier pause before starting new delivery work while a
// backup snapshot or restore is running. It never holds the gate during a
// network request.
func (n *Notifier) SetGate(gate *maintenance.Gate) {
	n.gate = gate
}

func NewNotifier(db *sql.DB, orderService *orders.Service, config Config) *Notifier {
	return &Notifier{
		db:            db,
		orders:        orderService,
		client:        NewClient(config.BotToken),
		recipients:    append([]int64(nil), config.AllowedUserIDs...),
		publicBaseURL: config.PublicBaseURL,
		logger:        log.Default(),
		wake:          make(chan struct{}, 1),
		interval:      outboxInterval,
		now:           time.Now,
	}
}

// EnqueueInTx inserts one outbox row per allowed recipient using the caller's
// transaction. Any error makes CreateOrder roll back the whole order.
func (n *Notifier) EnqueueInTx(tx *sql.Tx, orderID int64) error {
	for _, recipientID := range n.recipients {
		if _, err := tx.Exec(
			`INSERT INTO notification_outbox (order_id, recipient_id, job_token)
			 VALUES (?, ?, lower(hex(randomblob(16))))`,
			orderID,
			recipientID,
		); err != nil {
			return fmt.Errorf("enqueue notification for recipient %d: %w", recipientID, err)
		}
	}

	return nil
}

// Wake signals the worker without blocking the HTTP request. The buffered
// channel coalesces bursts, and the periodic ticker remains the safety net
// for lost signals and restarts.
func (n *Notifier) Wake() {
	select {
	case n.wake <- struct{}{}:
	default:
	}
}

func (n *Notifier) Run(ctx context.Context) {
	n.logger.Printf("telegram: starting notifier for %d recipient(s)", len(n.recipients))

	go n.checkBot(ctx)
	go newBot(n.client, n.recipients, n.logger).Run(ctx)

	n.process(ctx)

	ticker := time.NewTicker(n.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-n.wake:
		case <-ticker.C:
		}

		n.process(ctx)
	}
}

func (n *Notifier) checkBot(ctx context.Context) {
	username, err := n.client.GetMe(ctx)
	if err != nil {
		n.logger.Printf("telegram: bot check failed: %v", err)
		return
	}
	if username != "" {
		n.logger.Printf("telegram: connected as @%s", username)
	}
}

var errMaintenance = errors.New("maintenance in progress")

// enterRead protects one SQLite access. It must not be held across a
// SendMessage call: restore must be able to acquire the exclusive gate while
// the notifier waits for Telegram. If maintenance is already active, the
// caller leaves the job pending for the next cycle.
func (n *Notifier) enterRead() bool {
	return n.gate == nil || n.gate.TryRead()
}

func (n *Notifier) leaveRead() {
	if n.gate != nil {
		n.gate.ReadUnlock()
	}
}

func (n *Notifier) loadOrder(ctx context.Context, orderID int64) (orders.AdminOrderResponse, bool, error) {
	if !n.enterRead() {
		return orders.AdminOrderResponse{}, false, errMaintenance
	}
	defer n.leaveRead()

	order, err := n.orders.GetAdminOrder(orderID)
	return order, true, err
}

func (n *Notifier) deleteOutboxForOrder(ctx context.Context, orderID int64) bool {
	if !n.enterRead() {
		return false
	}
	defer n.leaveRead()

	if _, err := n.db.ExecContext(
		ctx,
		`DELETE FROM notification_outbox WHERE order_id = ?`,
		orderID,
	); err != nil {
		n.logger.Printf("telegram: delete outbox rows for order #%d: %v", orderID, err)
	}
	return true
}

func (n *Notifier) process(ctx context.Context) {
	if n.gate != nil && n.gate.Active() {
		return
	}

	processed := make(map[int64]struct{})

	for {
		if ctx.Err() != nil {
			return
		}
		if n.gate != nil && n.gate.Active() {
			return
		}

		jobs, err := n.fetchDue(ctx, outboxBatchSize)
		if errors.Is(err, errMaintenance) {
			return
		}
		if err != nil {
			n.logger.Printf("telegram: fetch outbox: %v", err)
			return
		}
		if len(jobs) == 0 {
			return
		}

		fresh := make([]outboxJob, 0, len(jobs))
		for _, job := range jobs {
			if _, exists := processed[job.ID]; exists {
				continue
			}
			processed[job.ID] = struct{}{}
			fresh = append(fresh, job)
		}
		if len(fresh) == 0 {
			return
		}

		n.deliver(ctx, fresh)

		if len(jobs) < outboxBatchSize {
			return
		}
	}
}

type outboxJob struct {
	ID          int64
	JobToken    string
	OrderID     int64
	RecipientID int64
	Attempts    int
}

func (n *Notifier) fetchDue(ctx context.Context, limit int) ([]outboxJob, error) {
	if !n.enterRead() {
		return nil, errMaintenance
	}
	defer n.leaveRead()

	rows, err := n.db.QueryContext(ctx, `
		SELECT id, job_token, order_id, recipient_id, attempts
		FROM notification_outbox
		WHERE status = 'pending' AND next_attempt_at <= ?
		ORDER BY next_attempt_at, id
		LIMIT ?
	`, n.now().UTC().Format(sqliteTimeLayout), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := make([]outboxJob, 0, limit)
	for rows.Next() {
		var job outboxJob
		if err := rows.Scan(
			&job.ID,
			&job.JobToken,
			&job.OrderID,
			&job.RecipientID,
			&job.Attempts,
		); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}

	return jobs, rows.Err()
}

type cachedOrder struct {
	parts   []string
	deleted bool
	err     error
}

func (n *Notifier) deliver(ctx context.Context, jobs []outboxJob) {
	cache := make(map[int64]cachedOrder)

	for _, job := range jobs {
		if ctx.Err() != nil {
			return
		}
		if n.gate != nil && n.gate.Active() {
			return
		}

		order, ok := cache[job.OrderID]
		if !ok {
			adminOrder, entered, err := n.loadOrder(ctx, job.OrderID)
			if !entered {
				return
			}

			switch {
			case err == nil:
				order = cachedOrder{parts: BuildOrderMessages(adminOrder, n.publicBaseURL)}
			case errors.Is(err, orders.ErrOrderNotFound):
				n.logger.Printf(
					"telegram: order #%d was deleted before notification; dropping pending tasks",
					job.OrderID,
				)
				if !n.deleteOutboxForOrder(ctx, job.OrderID) {
					return
				}
				order = cachedOrder{deleted: true}
			default:
				order = cachedOrder{err: err}
			}
			cache[job.OrderID] = order
		}

		if order.deleted {
			continue
		}
		if order.err != nil {
			n.logger.Printf("telegram: load order #%d: %v", job.OrderID, order.err)
			n.recordFailure(ctx, job, order.err, false, 0)
			continue
		}

		n.deliverJob(ctx, job, order.parts)
	}
}

func (n *Notifier) deliverJob(ctx context.Context, job outboxJob, parts []string) {
	for _, part := range parts {
		if ctx.Err() != nil {
			return
		}
		if n.gate != nil && n.gate.Active() {
			return
		}

		err := n.client.SendMessage(ctx, job.RecipientID, part)
		if err == nil {
			continue
		}

		if ctx.Err() != nil {
			// Shutdown interrupted the send; leave the row pending for the
			// next start instead of recording a misleading failure.
			return
		}

		permanent := !isTemporary(err)

		var apiErr *APIError
		if errors.As(err, &apiErr) {
			switch apiErr.Code {
			case 403:
				n.logger.Printf(
					"telegram: user %d has not started the bot or blocked it (order #%d)",
					job.RecipientID,
					job.OrderID,
				)
			case 400:
				n.logger.Printf(
					"telegram: message rejected for order #%d to user %d; check formatting",
					job.OrderID,
					job.RecipientID,
				)
			}
		}

		n.recordFailure(ctx, job, err, permanent, retryAfterOf(err))
		return
	}

	n.markSent(ctx, job)
}

func (n *Notifier) markSent(ctx context.Context, job outboxJob) {
	if job.JobToken == "" {
		n.logger.Printf("telegram: notification %d has no job identity; ignoring stale success", job.ID)
		return
	}
	if !n.enterRead() {
		return
	}
	defer n.leaveRead()

	result, err := n.db.ExecContext(ctx, `
		UPDATE notification_outbox
		SET status = 'sent', attempts = attempts + 1, sent_at = CURRENT_TIMESTAMP, last_error = ''
		WHERE id = ? AND job_token = ?
	`, job.ID, job.JobToken)
	if err != nil {
		n.logger.Printf("telegram: mark notification %d as sent: %v", job.ID, err)
		return
	}

	affected, err := result.RowsAffected()
	if err != nil {
		n.logger.Printf("telegram: check mark notification %d as sent: %v", job.ID, err)
		return
	}
	if affected == 0 {
		n.logger.Printf(
			"telegram: notification %d no longer matches job %s; ignoring stale success",
			job.ID,
			job.JobToken,
		)
	}
}

func (n *Notifier) recordFailure(
	ctx context.Context,
	job outboxJob,
	sendErr error,
	permanent bool,
	retryAfter time.Duration,
) {
	if job.JobToken == "" {
		n.logger.Printf("telegram: notification %d has no job identity; ignoring stale failure", job.ID)
		return
	}
	if !n.enterRead() {
		return
	}
	defer n.leaveRead()

	status := "pending"
	delay := retryAfter

	if permanent {
		status = "failed"
		delay = 0
	} else {
		backoff := retryDelay(job.Attempts + 1)
		if delay < backoff {
			delay = backoff
		}
	}

	nextAttempt := n.now().Add(delay).UTC().Format(sqliteTimeLayout)
	attempt := job.Attempts + 1

	result, err := n.db.ExecContext(ctx, `
		UPDATE notification_outbox
		SET attempts = attempts + 1,
		    status = ?,
		    last_error = ?,
		    next_attempt_at = ?
		WHERE id = ? AND job_token = ?
	`, status, truncate(sendErr.Error(), 500), nextAttempt, job.ID, job.JobToken)
	if err != nil {
		n.logger.Printf("telegram: record failed attempt for notification %d: %v", job.ID, err)
		return
	}

	affected, err := result.RowsAffected()
	if err != nil {
		n.logger.Printf("telegram: check failed attempt for notification %d: %v", job.ID, err)
		return
	}
	if affected == 0 {
		n.logger.Printf(
			"telegram: notification %d no longer matches job %s; ignoring stale failure",
			job.ID,
			job.JobToken,
		)
		return
	}

	if permanent {
		n.logger.Printf(
			"telegram: permanent delivery failure for order #%d to user %d on attempt %d: %v",
			job.OrderID,
			job.RecipientID,
			attempt,
			sendErr,
		)
		return
	}

	n.logger.Printf(
		"telegram: temporary delivery failure for order #%d to user %d on attempt %d: %v; next attempt at %s",
		job.OrderID,
		job.RecipientID,
		attempt,
		sendErr,
		nextAttempt,
	)
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}

	delay := 30 * time.Second
	for i := 1; i < attempt && delay < maxRetryDelay; i++ {
		delay *= 2
	}

	if delay > maxRetryDelay {
		delay = maxRetryDelay
	}

	return delay
}

func retryAfterOf(err error) time.Duration {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.RetryAfter
	}

	return 0
}
