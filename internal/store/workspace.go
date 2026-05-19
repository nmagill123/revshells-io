package store

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/noahmagill/webhook-rev-shell/internal/protocol"
	bolt "go.etcd.io/bbolt"
)

var (
	bucketWorkspaces              = []byte("workspaces")
	bucketWorkspaceBrowserTokens  = []byte("workspace_browser_tokens")
)

func (s *Store) initWorkspaceBuckets(tx *bolt.Tx) error {
	for _, b := range [][]byte{bucketWorkspaces, bucketWorkspaceBrowserTokens} {
		if _, err := tx.CreateBucketIfNotExists(b); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) PutWorkspace(w *protocol.Workspace) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(w)
		if err != nil {
			return err
		}
		return tx.Bucket(bucketWorkspaces).Put([]byte(w.ID), data)
	})
}

func (s *Store) GetWorkspace(id string) (*protocol.Workspace, error) {
	var w protocol.Workspace
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketWorkspaces).Get([]byte(id))
		if data == nil {
			return fmt.Errorf("workspace not found")
		}
		return json.Unmarshal(data, &w)
	})
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (s *Store) PutWorkspaceBrowserToken(bt *protocol.WorkspaceBrowserToken) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(bt)
		if err != nil {
			return err
		}
		return tx.Bucket(bucketWorkspaceBrowserTokens).Put([]byte(bt.Token), data)
	})
}

func (s *Store) GetWorkspaceBrowserToken(token string) (*protocol.WorkspaceBrowserToken, error) {
	var bt protocol.WorkspaceBrowserToken
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketWorkspaceBrowserTokens).Get([]byte(token))
		if data == nil {
			return fmt.Errorf("workspace browser token not found")
		}
		return json.Unmarshal(data, &bt)
	})
	if err != nil {
		return nil, err
	}
	return &bt, nil
}

func (s *Store) CountActiveSessionsForWorkspace(workspaceID string) (int, error) {
	n := 0
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketSessions).ForEach(func(_, v []byte) error {
			var sess protocol.Session
			if err := json.Unmarshal(v, &sess); err != nil {
				return nil
			}
			if sess.OwnerWorkspaceID != workspaceID {
				return nil
			}
			if sess.State == protocol.StateWaiting || sess.State == protocol.StateActive {
				n++
			}
			return nil
		})
	})
	return n, err
}

func (s *Store) ListSessionsByWorkspace(workspaceID string) ([]*protocol.Session, error) {
	var sessions []*protocol.Session
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketSessions).ForEach(func(_, v []byte) error {
			var sess protocol.Session
			if err := json.Unmarshal(v, &sess); err != nil {
				return nil
			}
			if sess.OwnerWorkspaceID == workspaceID {
				sessions = append(sessions, &sess)
			}
			return nil
		})
	})
	return sessions, err
}

func (s *Store) SessionOwnedBy(workspaceID, sessionID string) (bool, error) {
	sess, err := s.GetSession(sessionID)
	if err != nil {
		return false, err
	}
	return sess.OwnerWorkspaceID == workspaceID, nil
}

func (s *Store) PruneWorkspaceBrowserTokens() error {
	now := time.Now()
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketWorkspaceBrowserTokens)
		var toDelete [][]byte
		b.ForEach(func(k, v []byte) error {
			var bt protocol.WorkspaceBrowserToken
			if err := json.Unmarshal(v, &bt); err != nil {
				toDelete = append(toDelete, k)
				return nil
			}
			if now.After(bt.ExpiresAt) {
				toDelete = append(toDelete, k)
			}
			return nil
		})
		for _, k := range toDelete {
			b.Delete(k)
		}
		return nil
	})
}
