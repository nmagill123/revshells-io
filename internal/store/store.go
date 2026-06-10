package store

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/noahmagill/webhook-rev-shell/internal/protocol"
	bolt "go.etcd.io/bbolt"
)

var (
	bucketSessions      = []byte("sessions")
	bucketTargets       = []byte("targets")
	bucketTranscripts   = []byte("transcripts")
	bucketTokens        = []byte("tokens")
	bucketBrowserTokens = []byte("browser_tokens")
)

type Store struct {
	db *bolt.DB
}

func Open(path string) (*Store, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open bolt db: %w", err)
	}
	st := &Store{db: db}
	err = db.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{
			bucketSessions, bucketTargets, bucketTranscripts,
			bucketTokens, bucketBrowserTokens,
		} {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return st.initWorkspaceBuckets(tx)
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("init buckets: %w", err)
	}
	return st, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// --- Sessions ---

func (s *Store) PutSession(sess *protocol.Session) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(sess)
		if err != nil {
			return err
		}
		return tx.Bucket(bucketSessions).Put([]byte(sess.ID), data)
	})
}

func (s *Store) GetSession(id string) (*protocol.Session, error) {
	var sess protocol.Session
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketSessions).Get([]byte(id))
		if data == nil {
			return fmt.Errorf("session not found: %s", id)
		}
		return json.Unmarshal(data, &sess)
	})
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *Store) ListSessions() ([]*protocol.Session, error) {
	var sessions []*protocol.Session
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketSessions).ForEach(func(k, v []byte) error {
			var sess protocol.Session
			if err := json.Unmarshal(v, &sess); err != nil {
				return err
			}
			sessions = append(sessions, &sess)
			return nil
		})
	})
	return sessions, err
}

func (s *Store) DeleteSession(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketSessions).Delete([]byte(id))
	})
}

func (s *Store) TouchSession(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSessions)
		data := b.Get([]byte(id))
		if data == nil {
			return nil
		}
		var sess protocol.Session
		if err := json.Unmarshal(data, &sess); err != nil {
			return err
		}
		sess.LastActivity = time.Now()
		updated, err := json.Marshal(&sess)
		if err != nil {
			return err
		}
		return b.Put([]byte(id), updated)
	})
}

// --- Targets ---

func (s *Store) PutTarget(t *protocol.Target) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		key := t.SessionID + "/" + t.ID
		data, err := json.Marshal(t)
		if err != nil {
			return err
		}
		return tx.Bucket(bucketTargets).Put([]byte(key), data)
	})
}

func (s *Store) ListTargets(sessionID string) ([]*protocol.Target, error) {
	prefix := []byte(sessionID + "/")
	var targets []*protocol.Target
	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bucketTargets).Cursor()
		for k, v := c.Seek(prefix); k != nil && len(k) >= len(prefix) && string(k[:len(prefix)]) == string(prefix); k, v = c.Next() {
			var t protocol.Target
			if err := json.Unmarshal(v, &t); err != nil {
				return err
			}
			targets = append(targets, &t)
		}
		return nil
	})
	return targets, err
}

func (s *Store) DeleteTarget(sessionID, targetID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		key := sessionID + "/" + targetID
		return tx.Bucket(bucketTargets).Delete([]byte(key))
	})
}

func (s *Store) DeleteTargetsForSession(sessionID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return s.deleteTargetsForSessionTx(tx, sessionID)
	})
}

// --- Transcripts ---

func (s *Store) AppendTranscript(sessionID, targetID string, seq int, data []byte) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		key := fmt.Sprintf("%s/%s/%08d", sessionID, targetID, seq)
		return tx.Bucket(bucketTranscripts).Put([]byte(key), data)
	})
}

func (s *Store) GetTranscript(sessionID, targetID string) ([][]byte, error) {
	prefix := []byte(sessionID + "/" + targetID + "/")
	var chunks [][]byte
	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bucketTranscripts).Cursor()
		for k, v := c.Seek(prefix); k != nil && len(k) >= len(prefix) && string(k[:len(prefix)]) == string(prefix); k, v = c.Next() {
			cp := make([]byte, len(v))
			copy(cp, v)
			chunks = append(chunks, cp)
		}
		return nil
	})
	return chunks, err
}

func (s *Store) DeleteTranscriptsForSession(sessionID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return s.deleteTranscriptsForSessionTx(tx, sessionID)
	})
}

// --- Browser Tokens ---

func (s *Store) PutBrowserToken(bt *protocol.BrowserToken) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(bt)
		if err != nil {
			return err
		}
		return tx.Bucket(bucketBrowserTokens).Put([]byte(bt.Token), data)
	})
}

func (s *Store) GetBrowserToken(token string) (*protocol.BrowserToken, error) {
	var bt protocol.BrowserToken
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketBrowserTokens).Get([]byte(token))
		if data == nil {
			return fmt.Errorf("browser token not found")
		}
		return json.Unmarshal(data, &bt)
	})
	if err != nil {
		return nil, err
	}
	return &bt, nil
}

func (s *Store) DeleteBrowserToken(token string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketBrowserTokens).Delete([]byte(token))
	})
}

func (s *Store) DeleteBrowserTokensForSession(sessionID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return s.deleteBrowserTokensForSessionTx(tx, sessionID)
	})
}

// --- Expiry Sweep ---

func (s *Store) ExpireSessions(maxIdle time.Duration) ([]string, error) {
	now := time.Now()
	var expired []string
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSessions)
		return b.ForEach(func(k, v []byte) error {
			var sess protocol.Session
			if err := json.Unmarshal(v, &sess); err != nil {
				return nil
			}
			if sess.State == protocol.StateWaiting || sess.State == protocol.StateActive {
				if now.Sub(sess.LastActivity) > maxIdle {
					sess.State = protocol.StateExpired
					data, _ := json.Marshal(&sess)
					b.Put(k, data)
					expired = append(expired, sess.ID)
				}
			}
			return nil
		})
	})
	return expired, err
}

func (s *Store) PruneBrowserTokens() error {
	now := time.Now()
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketBrowserTokens)
		var toDelete [][]byte
		b.ForEach(func(k, v []byte) error {
			var bt protocol.BrowserToken
			if err := json.Unmarshal(v, &bt); err != nil {
				cp := make([]byte, len(k))
				copy(cp, k)
				toDelete = append(toDelete, cp)
				return nil
			}
			if now.After(bt.ExpiresAt) {
				cp := make([]byte, len(k))
				copy(cp, k)
				toDelete = append(toDelete, cp)
			}
			return nil
		})
		for _, k := range toDelete {
			b.Delete(k)
		}
		return nil
	})
}

// --- Operator Tokens ---

type OperatorToken struct {
	TokenHash   string    `json:"token_hash"`
	WorkspaceID string    `json:"workspace_id"`
	SessionID   string    `json:"session_id,omitempty"` // empty = workspace-scoped CLI
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func (s *Store) PutOperatorToken(t *OperatorToken) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(t)
		if err != nil {
			return err
		}
		return tx.Bucket(bucketTokens).Put([]byte(t.TokenHash), data)
	})
}

func (s *Store) GetOperatorToken(hash string) (*OperatorToken, error) {
	var t OperatorToken
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketTokens).Get([]byte(hash))
		if data == nil {
			return fmt.Errorf("operator token not found")
		}
		return json.Unmarshal(data, &t)
	})
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) PruneOperatorTokens() error {
	now := time.Now()
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketTokens)
		var toDelete [][]byte
		b.ForEach(func(k, v []byte) error {
			var t OperatorToken
			if err := json.Unmarshal(v, &t); err != nil {
				cp := make([]byte, len(k))
				copy(cp, k)
				toDelete = append(toDelete, cp)
				return nil
			}
			if now.After(t.ExpiresAt) {
				cp := make([]byte, len(k))
				copy(cp, k)
				toDelete = append(toDelete, cp)
			}
			return nil
		})
		for _, k := range toDelete {
			b.Delete(k)
		}
		return nil
	})
}

func (s *Store) DeleteOperatorTokensForSession(sessionID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return s.deleteOperatorTokensForSessionTx(tx, sessionID)
	})
}

func (s *Store) CleanupSessionArtifacts(sessionID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := s.deleteTargetsForSessionTx(tx, sessionID); err != nil {
			return err
		}
		if err := s.deleteTranscriptsForSessionTx(tx, sessionID); err != nil {
			return err
		}
		if err := s.deleteBrowserTokensForSessionTx(tx, sessionID); err != nil {
			return err
		}
		return s.deleteOperatorTokensForSessionTx(tx, sessionID)
	})
}

func (s *Store) deleteTargetsForSessionTx(tx *bolt.Tx, sessionID string) error {
	prefix := []byte(sessionID + "/")
	b := tx.Bucket(bucketTargets)
	c := b.Cursor()
	var keys [][]byte
	for k, _ := c.Seek(prefix); k != nil && len(k) >= len(prefix) && string(k[:len(prefix)]) == string(prefix); k, _ = c.Next() {
		cp := make([]byte, len(k))
		copy(cp, k)
		keys = append(keys, cp)
	}
	for _, k := range keys {
		if err := b.Delete(k); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) deleteTranscriptsForSessionTx(tx *bolt.Tx, sessionID string) error {
	prefix := []byte(sessionID + "/")
	b := tx.Bucket(bucketTranscripts)
	c := b.Cursor()
	var keys [][]byte
	for k, _ := c.Seek(prefix); k != nil && len(k) >= len(prefix) && string(k[:len(prefix)]) == string(prefix); k, _ = c.Next() {
		cp := make([]byte, len(k))
		copy(cp, k)
		keys = append(keys, cp)
	}
	for _, k := range keys {
		if err := b.Delete(k); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) deleteBrowserTokensForSessionTx(tx *bolt.Tx, sessionID string) error {
	b := tx.Bucket(bucketBrowserTokens)
	var toDelete [][]byte
	if err := b.ForEach(func(k, v []byte) error {
		var bt protocol.BrowserToken
		if err := json.Unmarshal(v, &bt); err != nil {
			return nil
		}
		if bt.SessionID == sessionID {
			cp := make([]byte, len(k))
			copy(cp, k)
			toDelete = append(toDelete, cp)
		}
		return nil
	}); err != nil {
		return err
	}
	for _, k := range toDelete {
		if err := b.Delete(k); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) deleteOperatorTokensForSessionTx(tx *bolt.Tx, sessionID string) error {
	b := tx.Bucket(bucketTokens)
	var toDelete [][]byte
	if err := b.ForEach(func(k, v []byte) error {
		var t OperatorToken
		if err := json.Unmarshal(v, &t); err != nil {
			return nil
		}
		if t.SessionID == sessionID {
			cp := make([]byte, len(k))
			copy(cp, k)
			toDelete = append(toDelete, cp)
		}
		return nil
	}); err != nil {
		return err
	}
	for _, k := range toDelete {
		if err := b.Delete(k); err != nil {
			return err
		}
	}
	return nil
}
