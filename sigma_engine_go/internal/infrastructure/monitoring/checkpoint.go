package monitoring

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
)

// CheckpointManager handles persistence of file read positions across restarts.
// It saves file offsets to a JSON file so the monitor can resume from where it left off.
type CheckpointManager struct {
	filePath string
	mu       sync.RWMutex
}

// Checkpoint represents the persisted state of all tracked files.
type Checkpoint struct {
	Files     map[string]FileCheckpoint `json:"files"`
	UpdatedAt time.Time                 `json:"updated_at"`
	Version   int                       `json:"version"` // For future compatibility
}

// FileCheckpoint represents the persisted state of a single tracked file.
type FileCheckpoint struct {
	Offset    int64     `json:"offset"`
	Inode     uint64    `json:"inode"`
	Size      int64     `json:"size"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NewCheckpointManager creates a new checkpoint manager.
// filePath is the path to the checkpoint JSON file.
func NewCheckpointManager(filePath string) *CheckpointManager {
	return &CheckpointManager{
		filePath: filePath,
	}
}

// Load loads the checkpoint from disk.
// Returns an empty checkpoint if the file doesn't exist.
func (cm *CheckpointManager) Load() (*Checkpoint, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Check if file exists
	if _, err := os.Stat(cm.filePath); os.IsNotExist(err) {
		logger.Debug("Checkpoint file does not exist, starting fresh")
		return &Checkpoint{
			Files:     make(map[string]FileCheckpoint),
			UpdatedAt: time.Now(),
			Version:   1,
		}, nil
	}

	// Read file
	data, err := os.ReadFile(cm.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read checkpoint file: %w", err)
	}

	// Parse JSON
	var checkpoint Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		logger.Warnf("Failed to parse checkpoint file, starting fresh: %v", err)
		return &Checkpoint{
			Files:     make(map[string]FileCheckpoint),
			UpdatedAt: time.Now(),
			Version:   1,
		}, nil
	}

	// Initialize map if nil
	if checkpoint.Files == nil {
		checkpoint.Files = make(map[string]FileCheckpoint)
	}

	logger.Infof("Loaded checkpoint with %d tracked files", len(checkpoint.Files))
	return &checkpoint, nil
}

// Save persists the current state of tracked files to disk.
func (cm *CheckpointManager) Save(trackedFiles map[string]*trackedFile) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Build checkpoint from tracked files
	checkpoint := Checkpoint{
		Files:     make(map[string]FileCheckpoint),
		UpdatedAt: time.Now(),
		Version:   1,
	}

	for path, tf := range trackedFiles {
		// Use relative path for portability
		relPath := path
		if absPath, err := filepath.Abs(path); err == nil {
			if rel, err := filepath.Rel(filepath.Dir(cm.filePath), absPath); err == nil {
				relPath = rel
			}
		}

		checkpoint.Files[relPath] = FileCheckpoint{
			Offset:    tf.Offset,
			Inode:     tf.Inode,
			Size:      tf.Size,
			UpdatedAt: time.Now(),
		}
	}

	// Ensure directory exists
	dir := filepath.Dir(cm.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create checkpoint directory: %w", err)
	}

	// Marshal to JSON with indentation for readability
	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	// Write atomically using temp file + rename
	tempPath := cm.filePath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write checkpoint temp file: %w", err)
	}

	if err := os.Rename(tempPath, cm.filePath); err != nil {
		os.Remove(tempPath) // Clean up temp file
		return fmt.Errorf("failed to rename checkpoint file: %w", err)
	}

	logger.Debugf("Saved checkpoint with %d files", len(checkpoint.Files))
	return nil
}

// ApplyToTrackedFiles applies loaded checkpoint data to tracked files.
// This restores file offsets from a previous run.
func (cm *CheckpointManager) ApplyToTrackedFiles(checkpoint *Checkpoint, trackedFiles map[string]*trackedFile) int {
	restored := 0

	for path, tf := range trackedFiles {
		// Try to find matching checkpoint entry
		relPath := path
		if absPath, err := filepath.Abs(path); err == nil {
			if rel, err := filepath.Rel(filepath.Dir(cm.filePath), absPath); err == nil {
				relPath = rel
			}
		}

		if cp, exists := checkpoint.Files[relPath]; exists {
			// Verify inode matches (file hasn't been rotated/replaced)
			if cp.Inode == tf.Inode || cp.Inode == 0 {
				// Verify offset is valid (file hasn't been truncated)
				if cp.Offset <= tf.Size {
					tf.Offset = cp.Offset
					restored++
					logger.Debugf("Restored offset for %s: %d bytes", path, cp.Offset)
				} else {
					logger.Warnf("Checkpoint offset %d exceeds current file size %d for %s, starting from beginning",
						cp.Offset, tf.Size, path)
				}
			} else {
				logger.Infof("File %s has different inode (was %d, now %d), starting from beginning",
					path, cp.Inode, tf.Inode)
			}
		}
	}

	return restored
}

// FilePath returns the checkpoint file path.
func (cm *CheckpointManager) FilePath() string {
	return cm.filePath
}
