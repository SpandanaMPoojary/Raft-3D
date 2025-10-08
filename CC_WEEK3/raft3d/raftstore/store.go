package raftstore

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hashicorp/raft"
)

type RaftStore struct {
	raft *raft.Raft
	fsm  *FSM
	mu   sync.RWMutex
}

func NewRaftStore(raft *raft.Raft, fsm *FSM) *RaftStore {
	return &RaftStore{
		raft: raft,
		fsm:  fsm,
	}
}

// Printer methods
func (s *RaftStore) CreatePrinter(p Printer) error {
	return s.applyCommand("ADD_PRINTER", p)
}

func (s *RaftStore) DeletePrinter(id string) error {
	return s.applyCommand("DELETE_PRINTER", id)
}

func (s *RaftStore) GetPrinters() []Printer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return mapToSlice(s.fsm.Printers)
}

func (s *RaftStore) GetPrinter(id string) (Printer, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, exists := s.fsm.Printers[id]
	return p, exists
}

// Filament methods
func (s *RaftStore) CreateFilament(f Filament) error {
	return s.applyCommand("ADD_FILAMENT", f)
}

func (s *RaftStore) GetFilaments() []Filament {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return mapToSlice(s.fsm.Filaments)
}

func (s *RaftStore) GetFilament(id string) (Filament, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, exists := s.fsm.Filaments[id]
	return f, exists
}

func (s *RaftStore) ReduceFilamentWeight(filamentID string, weight int) error {
	cmd := map[string]interface{}{
		"filament_id": filamentID,
		"weight":      weight,
	}
	return s.applyCommand("REDUCE_FILAMENT", cmd)
}

func (s *RaftStore) GetPendingFilamentUsage(filamentID string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total := 0
	for _, job := range s.fsm.PrintJobs {
		if job.FilamentID == filamentID &&
			(job.Status == "Queued" || job.Status == "Running") {
			total += job.PrintWeightInGrams
		}
	}
	return total
}

// PrintJob methods
func (s *RaftStore) CreatePrintJob(j PrintJob) error {
	return s.applyCommand("ADD_JOB", j)
}

func (s *RaftStore) GetPrintJobs() []PrintJob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return mapToSlice(s.fsm.PrintJobs)
}

func (s *RaftStore) GetPrintJob(id string) (PrintJob, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, exists := s.fsm.PrintJobs[id]
	return j, exists
}

func (s *RaftStore) UpdateJobStatus(jobID string, status string) error {
	cmd := map[string]string{
		"job_id": jobID,
		"status": status,
	}
	return s.applyCommand("UPDATE_JOB_STATUS", cmd)
}

// Raft operations
func (s *RaftStore) Raft() *raft.Raft {
	return s.raft
}

// Helper functions
func (s *RaftStore) applyCommand(op string, payload interface{}) error {
	cmd := map[string]interface{}{
		"op":      op,
		"payload": payload,
	}
	data, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	return s.raft.Apply(data, 10*time.Second).Error()
}

func mapToSlice[T any](m map[string]T) []T {
	slice := make([]T, 0, len(m))
	for _, v := range m {
		slice = append(slice, v)
	}
	return slice
}

// week3 adding this
func (rs *RaftStore) handleAddFilament(c *gin.Context) {
	var filament Filament
	if err := c.ShouldBindJSON(&filament); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cmd := map[string]interface{}{
		"op":      "ADD_FILAMENT",
		"payload": filament,
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to marshal command"})
		return
	}

	future := rs.raft.Apply(data, 5*time.Second)
	if err := future.Error(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "Filament added"})
}
func (rs *RaftStore) handleAddPrinter(c *gin.Context) {
	var printer Printer
	if err := c.ShouldBindJSON(&printer); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := rs.CreatePrinter(printer); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "Printer added"})
}
func (rs *RaftStore) handleUpdatePrintJobStatus(c *gin.Context) {
	jobID := c.Param("id")
	var req struct {
		Status string `json:"status"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	err := rs.UpdateJobStatus(jobID, req.Status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "print job status updated"})
}
