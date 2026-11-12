package nanopay

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
)

// ErrAlreadyClaimed is returned when a block hash or an order id has already
// paid for a request. One block pays for one order, once.
var ErrAlreadyClaimed = errors.New("nanopay: payment already claimed")

var blockHashPattern = regexp.MustCompile(`^[0-9A-F]{64}$`)

// ClaimStore records which block paid which order. Claim must be atomic: it
// either records both keys or returns an error and records nothing.
type ClaimStore interface {
	Claim(blockHash, orderID string) error
}

// FileClaimStore keeps claims in an append-only file, one
// "<BLOCK_HASH> <order_id>" line each, written and fsynced before Claim
// returns. The file is read back on open, so a restart does not make an old
// block spendable again.
type FileClaimStore struct {
	mu     sync.Mutex
	f      *os.File
	blocks map[string]struct{}
	orders map[string]struct{}
}

// OpenFileClaimStore opens (or creates, mode 0600) the claims file at path and
// loads every claim already in it. A malformed line is an error, not a skip.
func OpenFileClaimStore(path string) (*FileClaimStore, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	s := &FileClaimStore{f: f, blocks: map[string]struct{}{}, orders: map[string]struct{}{}}
	sc := bufio.NewScanner(f)
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		fields := strings.Fields(text)
		if len(fields) != 2 || !blockHashPattern.MatchString(fields[0]) || !ValidOrderID(fields[1]) {
			f.Close()
			return nil, fmt.Errorf("nanopay: claims file %s line %d is malformed", path, line)
		}
		s.blocks[fields[0]] = struct{}{}
		s.orders[fields[1]] = struct{}{}
	}
	if err := sc.Err(); err != nil {
		f.Close()
		return nil, err
	}
	return s, nil
}

// Claim records blockHash -> orderID, refusing a block or an order that was
// claimed before.
func (s *FileClaimStore) Claim(blockHash, orderID string) error {
	blockHash = strings.ToUpper(blockHash)
	if !blockHashPattern.MatchString(blockHash) || !ValidOrderID(orderID) {
		return errors.New("nanopay: malformed claim")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.blocks[blockHash]; ok {
		return ErrAlreadyClaimed
	}
	if _, ok := s.orders[orderID]; ok {
		return ErrAlreadyClaimed
	}
	if _, err := s.f.WriteString(blockHash + " " + orderID + "\n"); err != nil {
		return err
	}
	if err := s.f.Sync(); err != nil {
		return err
	}
	s.blocks[blockHash] = struct{}{}
	s.orders[orderID] = struct{}{}
	return nil
}

// Close closes the claims file.
func (s *FileClaimStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.f.Close()
}
