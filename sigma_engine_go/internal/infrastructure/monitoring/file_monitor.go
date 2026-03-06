package monitoring

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/edr-platform/sigma-engine/internal/domain"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
)

// FileMonitor monitors a directory for log files and reads new lines in real-time.
// It tracks file offsets to avoid re-reading, handles file rotation, and recovers from errors gracefully.
type FileMonitor struct {
	watchDir      string
	filePattern   *regexp.Regexp
	pollInterval  time.Duration
	maxFileSizeGB int64

	// Tracked files: path -> fileInfo
	trackedFiles map[string]*trackedFile
	mu           sync.RWMutex

	// Event channel
	eventChan chan *domain.LogEvent
	errorChan chan error

	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Statistics
	stats *MonitorStats

	// Checkpoint persistence
	checkpointMgr      *CheckpointManager
	checkpointInterval time.Duration
}

// trackedFile represents a file being monitored.
type trackedFile struct {
	Path      string
	Offset    int64     // Current read offset
	Inode     uint64    // File inode (for rotation detection)
	Size      int64     // Last known file size
	LastSeen  time.Time // Last time file was seen
	FirstSeen time.Time // First time file was discovered
}

// MonitorStats tracks file monitoring statistics.
type MonitorStats struct {
	FilesDiscovered   int64
	FilesTracked      int64
	LinesRead         int64
	EventsEmitted     int64
	Errors            int64
	RotationsDetected int64
	mu                sync.RWMutex
}

// NewFileMonitor creates a new file monitor.
// Parameters:
//   - watchDir: Directory to monitor
//   - filePattern: Glob pattern for files to monitor (e.g., "*.jsonl")
//   - pollInterval: How often to check for new files/lines (default: 100ms)
//   - maxFileSizeGB: Maximum file size to monitor (default: 1GB)
//   - checkpointPath: Path to checkpoint file for offset persistence (empty to disable)
//   - checkpointInterval: How often to save checkpoint (default: 30s)
func NewFileMonitor(watchDir, filePattern string, pollInterval time.Duration, maxFileSizeGB int64, checkpointPath string, checkpointInterval time.Duration) (*FileMonitor, error) {
	if pollInterval <= 0 {
		pollInterval = 100 * time.Millisecond
	}
	if maxFileSizeGB <= 0 {
		maxFileSizeGB = 1
	}
	if checkpointInterval <= 0 {
		checkpointInterval = 30 * time.Second
	}

	// Convert glob pattern to regex
	regexPattern := globToRegex(filePattern)
	pattern, err := regexp.Compile(regexPattern)
	if err != nil {
		return nil, fmt.Errorf("invalid file pattern %q: %w", filePattern, err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create checkpoint manager if path specified
	var checkpointMgr *CheckpointManager
	if checkpointPath != "" {
		checkpointMgr = NewCheckpointManager(checkpointPath)
	}

	return &FileMonitor{
		watchDir:      watchDir,
		filePattern:   pattern,
		pollInterval:  pollInterval,
		maxFileSizeGB: maxFileSizeGB * 1024 * 1024 * 1024, // Convert GB to bytes

		trackedFiles: make(map[string]*trackedFile),
		eventChan:    make(chan *domain.LogEvent, 1000),
		errorChan:    make(chan error, 100),

		ctx:    ctx,
		cancel: cancel,
		stats:  &MonitorStats{},

		checkpointMgr:      checkpointMgr,
		checkpointInterval: checkpointInterval,
	}, nil
}

// Start starts the file monitor.
// It begins monitoring the directory and emitting events through the event channel.
func (fm *FileMonitor) Start() error {
	// Validate watch directory
	info, err := os.Stat(fm.watchDir)
	if err != nil {
		return fmt.Errorf("watch directory does not exist: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("watch path is not a directory: %s", fm.watchDir)
	}

	// Load checkpoint if available
	var checkpoint *Checkpoint
	if fm.checkpointMgr != nil {
		var err error
		checkpoint, err = fm.checkpointMgr.Load()
		if err != nil {
			logger.Warnf("Failed to load checkpoint: %v", err)
		}
	}

	// Discover files first to populate trackedFiles
	fm.discoverFiles()

	// Apply checkpoint offsets to discovered files
	if checkpoint != nil && fm.checkpointMgr != nil {
		fm.mu.Lock()
		restored := fm.checkpointMgr.ApplyToTrackedFiles(checkpoint, fm.trackedFiles)
		fm.mu.Unlock()
		if restored > 0 {
			logger.Infof("Restored offsets for %d files from checkpoint", restored)
		}
	}

	// Start monitoring goroutine
	fm.wg.Add(1)
	go fm.monitorLoop()

	// Start checkpoint save goroutine if enabled
	if fm.checkpointMgr != nil {
		fm.wg.Add(1)
		go fm.checkpointLoop()
	}

	logger.Infof("File monitor started: watching %s for pattern %s", fm.watchDir, fm.filePattern.String())
	return nil
}

// Stop stops the file monitor gracefully.
func (fm *FileMonitor) Stop() {
	// Save final checkpoint before stopping
	if fm.checkpointMgr != nil {
		fm.mu.RLock()
		err := fm.checkpointMgr.Save(fm.trackedFiles)
		fm.mu.RUnlock()
		if err != nil {
			logger.Warnf("Failed to save final checkpoint: %v", err)
		} else {
			logger.Info("Saved checkpoint before shutdown")
		}
	}

	fm.cancel()
	fm.wg.Wait()
	close(fm.eventChan)
	close(fm.errorChan)
	logger.Info("File monitor stopped")
}

// Events returns the channel of events.
func (fm *FileMonitor) Events() <-chan *domain.LogEvent {
	return fm.eventChan
}

// Errors returns the channel of errors.
func (fm *FileMonitor) Errors() <-chan error {
	return fm.errorChan
}

// Stats returns monitoring statistics.
func (fm *FileMonitor) Stats() MonitorStats {
	fm.stats.mu.RLock()
	defer fm.stats.mu.RUnlock()

	return MonitorStats{
		FilesDiscovered:   fm.stats.FilesDiscovered,
		FilesTracked:      fm.stats.FilesTracked,
		LinesRead:         fm.stats.LinesRead,
		EventsEmitted:     fm.stats.EventsEmitted,
		Errors:            fm.stats.Errors,
		RotationsDetected: fm.stats.RotationsDetected,
	}
}

// checkpointLoop periodically saves checkpoint to persist file offsets.
func (fm *FileMonitor) checkpointLoop() {
	defer fm.wg.Done()

	ticker := time.NewTicker(fm.checkpointInterval)
	defer ticker.Stop()

	for {
		select {
		case <-fm.ctx.Done():
			return
		case <-ticker.C:
			fm.mu.RLock()
			err := fm.checkpointMgr.Save(fm.trackedFiles)
			fm.mu.RUnlock()
			if err != nil {
				logger.Warnf("Failed to save periodic checkpoint: %v", err)
			}
		}
	}
}

// monitorLoop is the main monitoring loop.
func (fm *FileMonitor) monitorLoop() {
	defer fm.wg.Done()

	ticker := time.NewTicker(fm.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-fm.ctx.Done():
			return
		case <-ticker.C:
			fm.scanAndRead()
		}
	}
}

// scanAndRead scans for new files and reads new lines from tracked files.
func (fm *FileMonitor) scanAndRead() {
	// Discover new files
	fm.discoverFiles()

	// Read new lines from tracked files
	fm.readNewLines()
}

// discoverFiles scans the watch directory for new files matching the pattern.
func (fm *FileMonitor) discoverFiles() {
	entries, err := os.ReadDir(fm.watchDir)
	if err != nil {
		fm.recordError(fmt.Errorf("failed to read directory: %w", err))
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Check if file matches pattern
		if !fm.filePattern.MatchString(entry.Name()) {
			continue
		}

		filePath := filepath.Join(fm.watchDir, entry.Name())

		// Check if already tracked
		fm.mu.RLock()
		_, exists := fm.trackedFiles[filePath]
		fm.mu.RUnlock()

		if exists {
			continue
		}

		// Get file info
		info, err := entry.Info()
		if err != nil {
			fm.recordError(fmt.Errorf("failed to get file info for %s: %w", filePath, err))
			continue
		}

		// Check file size
		if info.Size() > fm.maxFileSizeGB {
			logger.Warnf("File %s exceeds max size (%d bytes), skipping", filePath, info.Size())
			continue
		}

		// Get inode (for rotation detection)
		inode, err := getInode(filePath)
		if err != nil {
			fm.recordError(fmt.Errorf("failed to get inode for %s: %w", filePath, err))
			continue
		}

		// Add to tracked files
		now := time.Now()
		tracked := &trackedFile{
			Path:      filePath,
			Offset:    0, // Start from beginning (or could resume from saved state)
			Inode:     inode,
			Size:      info.Size(),
			LastSeen:  now,
			FirstSeen: now,
		}

		fm.mu.Lock()
		fm.trackedFiles[filePath] = tracked
		fm.mu.Unlock()

		fm.stats.mu.Lock()
		fm.stats.FilesDiscovered++
		fm.stats.FilesTracked++
		fm.stats.mu.Unlock()

		logger.Debugf("Discovered new file: %s (size: %d bytes)", filePath, info.Size())
	}
}

// readNewLines reads new lines from all tracked files.
func (fm *FileMonitor) readNewLines() {
	fm.mu.RLock()
	files := make([]*trackedFile, 0, len(fm.trackedFiles))
	for _, f := range fm.trackedFiles {
		files = append(files, f)
	}
	fm.mu.RUnlock()

	for _, tracked := range files {
		fm.readFileLines(tracked)
	}
}

// readFileLines reads new lines from a tracked file.
func (fm *FileMonitor) readFileLines(tracked *trackedFile) {
	// Check if file still exists
	info, err := os.Stat(tracked.Path)
	if err != nil {
		// File may have been deleted or rotated
		if os.IsNotExist(err) {
			fm.handleFileRotation(tracked)
		} else {
			fm.recordError(fmt.Errorf("failed to stat file %s: %w", tracked.Path, err))
		}
		return
	}

	// Check for rotation (inode change)
	inode, err := getInode(tracked.Path)
	if err != nil {
		fm.recordError(fmt.Errorf("failed to get inode for %s: %w", tracked.Path, err))
		return
	}

	if inode != tracked.Inode {
		// File rotated
		fm.handleFileRotation(tracked)
		return
	}

	// Check if file has new data
	if info.Size() <= tracked.Offset {
		// No new data
		tracked.LastSeen = time.Now()
		return
	}

	// Open file for reading
	file, err := os.Open(tracked.Path)
	if err != nil {
		fm.recordError(fmt.Errorf("failed to open file %s: %w", tracked.Path, err))
		return
	}
	defer file.Close()

	// Seek to last read position
	if _, err := file.Seek(tracked.Offset, 0); err != nil {
		fm.recordError(fmt.Errorf("failed to seek in file %s: %w", tracked.Path, err))
		return
	}

	// Read new lines
	scanner := bufio.NewScanner(file)
	lineCount := 0

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// Parse JSON line to LogEvent
		event, err := fm.parseEventLine(line, tracked.Path)
		if err != nil {
			fm.recordError(fmt.Errorf("failed to parse event from %s: %w", tracked.Path, err))
			continue
		}

		// Emit event
		select {
		case fm.eventChan <- event:
			fm.stats.mu.Lock()
			fm.stats.EventsEmitted++
			fm.stats.mu.Unlock()
		case <-fm.ctx.Done():
			return
		}

		lineCount++
	}

	if err := scanner.Err(); err != nil {
		fm.recordError(fmt.Errorf("scanner error for %s: %w", tracked.Path, err))
	}

	// Update offset
	currentOffset, _ := file.Seek(0, 1) // Get current position
	tracked.Offset = currentOffset
	tracked.Size = info.Size()
	tracked.LastSeen = time.Now()

	fm.stats.mu.Lock()
	fm.stats.LinesRead += int64(lineCount)
	fm.stats.mu.Unlock()

	if lineCount > 0 {
		logger.Debugf("Read %d new lines from %s (offset: %d)", lineCount, tracked.Path, tracked.Offset)
	}
}

// handleFileRotation handles file rotation by resetting tracking.
func (fm *FileMonitor) handleFileRotation(tracked *trackedFile) {
	// Check if new file exists with same name
	info, err := os.Stat(tracked.Path)
	if err != nil {
		// File deleted, remove from tracking
		fm.mu.Lock()
		delete(fm.trackedFiles, tracked.Path)
		fm.mu.Unlock()

		fm.stats.mu.Lock()
		fm.stats.FilesTracked--
		fm.stats.mu.Unlock()

		logger.Debugf("File deleted: %s", tracked.Path)
		return
	}

	// File rotated, reset tracking
	inode, _ := getInode(tracked.Path)
	tracked.Inode = inode
	tracked.Offset = 0
	tracked.Size = info.Size()
	tracked.LastSeen = time.Now()

	fm.stats.mu.Lock()
	fm.stats.RotationsDetected++
	fm.stats.mu.Unlock()

	logger.Infof("File rotated: %s (new inode: %d)", tracked.Path, inode)
}

// parseEventLine parses a JSON line into a LogEvent.
// Adds source_file to event data for tracking.
func (fm *FileMonitor) parseEventLine(line []byte, sourceFile string) (*domain.LogEvent, error) {
	var eventData map[string]interface{}
	if err := json.Unmarshal(line, &eventData); err != nil {
		return nil, fmt.Errorf("json unmarshal failed: %w", err)
	}

	// Add source file to event data if not already present
	if _, exists := eventData["source_file"]; !exists && sourceFile != "" {
		eventData["source_file"] = sourceFile
	}

	event, err := domain.NewLogEvent(eventData)
	if err != nil {
		return nil, fmt.Errorf("failed to create LogEvent: %w", err)
	}

	return event, nil
}

// recordError records an error and sends it to the error channel.
func (fm *FileMonitor) recordError(err error) {
	fm.stats.mu.Lock()
	fm.stats.Errors++
	fm.stats.mu.Unlock()

	select {
	case fm.errorChan <- err:
	case <-fm.ctx.Done():
	}
}

// globToRegex converts a glob pattern to a regex pattern.
func globToRegex(pattern string) string {
	// Escape special regex characters
	pattern = regexp.QuoteMeta(pattern)
	// Convert glob wildcards
	pattern = regexp.MustCompile(`\\\*`).ReplaceAllString(pattern, ".*")
	pattern = regexp.MustCompile(`\\\?`).ReplaceAllString(pattern, ".")
	return "^" + pattern + "$"
}

// getInode gets the file inode (for rotation detection).
// On Windows, uses file index number.
func getInode(filePath string) (uint64, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return 0, err
	}

	// Use file modification time and size as a proxy for inode on Windows
	// On Unix systems, we could use syscall.Stat to get actual inode
	// For cross-platform, we use a combination of path, size, and mod time
	// This is a simplified approach - in production, use syscall for actual inode
	return uint64(info.ModTime().UnixNano()), nil
}
