/*
Package wuid provides WUID, an extremely fast unique number generator. It is 10-135 times faster
than UUID and 4600 times faster than generating unique numbers with Redis.

WUID generates unique 64-bit integers in sequence. The high 28 bits are loaded from a data store.
By now, Redis, MySQL, and MongoDB are supported.
*/
package wuid

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/edwingeng/wuid/internal"
	_ "github.com/go-sql-driver/mysql" //...
)

/*
Logger includes internal.Logger, while internal.Logger includes:
	Info(args ...interface{})
	Warn(args ...interface{})
*/
type Logger interface {
	internal.Logger
}

// WUID is an extremely fast unique number generator.
type WUID struct {
	w *internal.WUID
}

// NewWUID creates a new WUID instance.
func NewWUID(tag string, logger Logger, opts ...Option) *WUID {
	var opts2 []internal.Option
	for _, opt := range opts {
		opts2 = append(opts2, internal.Option(opt))
	}
	return &WUID{w: internal.NewWUID(tag, logger, opts2...)}
}

// Next returns the next unique number.
func (this *WUID) Next() uint64 {
	return this.w.Next()
}

type NewDB func() (client *sql.DB, autoDisconnect bool, err error)

// LoadH28FromMysql adds 1 to a specific number in your MySQL, fetches its new value, and then
// sets that as the high 28 bits of the unique numbers that Next generates.
func (this *WUID) LoadH28FromMysql(newDB NewDB, table string) error {
	if len(table) == 0 {
		return errors.New("table cannot be empty. tag: " + this.w.Tag)
	}

	db, autoDisconnect, err := newDB()
	if err != nil {
		return err
	}
	if autoDisconnect {
		defer func() {
			_ = db.Close()
		}()
	}

	result, err := db.Exec(fmt.Sprintf("REPLACE INTO %s (x) VALUES (0)", table))
	if err != nil {
		return err
	}
	lastInsertedID, err := result.LastInsertId()
	if err != nil {
		return err
	}
	h28 := uint64(lastInsertedID)
	if err = this.w.VerifyH28(h28); err != nil {
		return err
	}

	this.w.Reset(h28 << 36)
	this.w.Logger.Info(fmt.Sprintf("<wuid> new h28: %d. tag: %s", h28, this.w.Tag))

	this.w.Lock()
	defer this.w.Unlock()

	if this.w.Renew != nil {
		return nil
	}
	this.w.Renew = func() error {
		return this.LoadH28FromMysql(newDB, table)
	}

	return nil
}

// RenewNow reacquires the high 28 bits from your data store immediately
func (this *WUID) RenewNow() error {
	return this.w.RenewNow()
}

// Option should never be used directly.
type Option internal.Option

// WithSection adds a section ID to the generated numbers. The section ID must be in between [1, 15].
// It occupies the highest 4 bits of the numbers.
func WithSection(section uint8) Option {
	return Option(internal.WithSection(section))
}

// WithH28Verifier sets your own h28 verifier
func WithH28Verifier(cb func(h28 uint64) error) Option {
	return Option(internal.WithH28Verifier(cb))
}
