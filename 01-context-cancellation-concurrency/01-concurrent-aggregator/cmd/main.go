package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"golang.org/x/sync/errgroup"
)

type UserAggregator struct {
	timeout        time.Duration
	logger         *slog.Logger
	profileService *ProfileService
	orderService   *OrderService
}

func (a *UserAggregator) Aggregate(ctx context.Context, id int) (user, error) {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)
	var orders int
	var name string

	g.Go(func() error {
		result, err := a.orderService.FetchOrders(ctx, id)
		if err != nil {
			a.logger.Error("OrderService failed",
				"error", err,
				"user_id", id,
				slog.Group("context",
					"err_type", err.Error(),
					"is_timeout", errors.Is(err, context.DeadlineExceeded),
					"is_canceled", errors.Is(err, context.Canceled),
				),
			)
			return fmt.Errorf("order service: %w", err)
		}
		orders = result
		return nil
	})

	g.Go(func() error {
		result, err := a.profileService.FetchProfile(ctx, id)
		if err != nil {
			a.logger.Error("ProfileService failed",
				"error", err,
				"user_id", id,
				slog.Group("context",
					"err_type", err.Error(),
					"is_timeout", errors.Is(err, context.DeadlineExceeded),
					"is_canceled", errors.Is(err, context.Canceled),
				),
			)
			return fmt.Errorf("profile service: %w", err)
		}
		name = result
		return nil
	})

	if err := g.Wait(); err != nil {
		return user{}, err
	}

	return user{name: name, orders: orders}, nil
}

type Option func(*UserAggregator)

func WithTimeout(t time.Duration) Option {
	return func(a *UserAggregator) {
		a.timeout = t
	}
}

func WithPService(s *ProfileService) Option {
	return func(a *UserAggregator) {
		a.profileService = s
	}
}

func WithOService(s *OrderService) Option {
	return func(a *UserAggregator) {
		a.orderService = s

	}
}

func WithLogger(log *slog.Logger) Option {
	return func(a *UserAggregator) {
		a.logger = log
	}
}

func New(options ...Option) *UserAggregator {
	ua := &UserAggregator{
		timeout: 10 * time.Second,
		logger:  slog.Default(),
	}

	for _, opt := range options {
		opt(ua)
	}

	return ua
}

type ProfileService struct {
	users map[int]string
	delay time.Duration
	flag  bool
}

func NewProfileService(delay time.Duration, flag bool) *ProfileService {
	return &ProfileService{
		users: map[int]string{
			123: "Alice",
			12:  "Egor",
			13:  "Anna",
		},
		delay: delay,
		flag:  flag,
	}
}

func (s *ProfileService) FetchProfile(ctx context.Context, id int) (string, error) {
	if s.flag == true {
		return "", errors.New("flag was seted to true")
	}

	select {
	case <-time.After(s.delay):
		name, ok := s.users[id]
		if !ok {
			return "", errors.New("user not found")
		}

		return name, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

type OrderService struct {
	orders map[int]int
	delay  time.Duration
}

func NewOrderService(delay time.Duration) *OrderService {
	return &OrderService{
		orders: map[int]int{
			123: 5,
			12:  0,
			13:  124,
		},
		delay: delay,
	}
}

func (s *OrderService) FetchOrders(ctx context.Context, id int) (int, error) {
	select {
	case <-time.After(s.delay):
		orders, ok := s.orders[id]
		if !ok {
			return 0, errors.New("orders not found")
		}

		return orders, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

type user struct {
	name   string
	orders int
}

func main() {
	logger := slog.New(
		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}),
	)

	userAggregator := New(
		WithTimeout(10*time.Second),
		WithLogger(logger),
		WithPService(NewProfileService(300*time.Millisecond, true)),
		WithOService(NewOrderService(5*time.Second)),
	)

	ctx := context.Background()

	result, err := userAggregator.Aggregate(ctx, 123)
	if err != nil {
		return
	}

	fmt.Print(result)
}
