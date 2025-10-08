package raftstore

import (
	"encoding/json"
	"io"
	"log"
	"sync"

	"github.com/hashicorp/raft"
)

type FSM struct {
	mu        sync.Mutex
	Printers  map[string]Printer
	Filaments map[string]Filament
	PrintJobs map[string]PrintJob
}

type Printer struct {
	ID      string `json:"id"`
	Company string `json:"company"`
	Model   string `json:"model"`
}

type Filament struct {
	ID                     string `json:"id"`
	Type                   string `json:"type"`
	Color                  string `json:"color"`
	TotalWeightInGrams     int    `json:"total_weight_in_grams"`
	RemainingWeightInGrams int    `json:"remaining_weight_in_grams"`
}

type PrintJob struct {
	ID                 string `json:"id"`
	PrinterID          string `json:"printer_id"`
	FilamentID         string `json:"filament_id"`
	FilePath           string `json:"filepath"`
	PrintWeightInGrams int    `json:"print_weight_in_grams"`
	Status             string `json:"status"`
}

func (f *FSM) Apply(logEntry *raft.Log) interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()

	var cmd struct {
		Op      string          `json:"op"`
		Payload json.RawMessage `json:"payload"`
	}

	if err := json.Unmarshal(logEntry.Data, &cmd); err != nil {
		log.Printf("Failed to parse command: %v", err)
		return nil
	}

	switch cmd.Op {
	case "ADD_PRINTER":
		var p Printer
		if err := json.Unmarshal(cmd.Payload, &p); err != nil {
			return err
		}
		f.Printers[p.ID] = p

	case "DELETE_PRINTER":
		var printerID string
		if err := json.Unmarshal(cmd.Payload, &printerID); err != nil {
			return err
		}
		delete(f.Printers, printerID)

	case "ADD_FILAMENT":
		var fl Filament
		if err := json.Unmarshal(cmd.Payload, &fl); err != nil {
			return err
		}
		f.Filaments[fl.ID] = fl

	case "ADD_JOB":
		var job PrintJob
		if err := json.Unmarshal(cmd.Payload, &job); err != nil {
			return err
		}
		f.PrintJobs[job.ID] = job

	case "UPDATE_JOB_STATUS":
		var update struct {
			JobID  string `json:"job_id"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal(cmd.Payload, &update); err != nil {
			return err
		}
		if job, exists := f.PrintJobs[update.JobID]; exists {
			job.Status = update.Status
			f.PrintJobs[update.JobID] = job
		}

	case "REDUCE_FILAMENT":
		var payload struct {
			FilamentID string `json:"filament_id"`
			Weight     int    `json:"weight"`
		}
		if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
			return err
		}
		if filament, exists := f.Filaments[payload.FilamentID]; exists {
			filament.RemainingWeightInGrams -= payload.Weight
			if filament.RemainingWeightInGrams < 0 {
				filament.RemainingWeightInGrams = 0
			}
			f.Filaments[payload.FilamentID] = filament
		}
	}

	return nil
}

func (f *FSM) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &snapshot{store: f}, nil
}

func (f *FSM) Restore(rc io.ReadCloser) error {
	return json.NewDecoder(rc).Decode(f)
}

type snapshot struct {
	store *FSM
}

func (s *snapshot) Persist(sink raft.SnapshotSink) error {
	data, err := json.Marshal(s.store)
	if err != nil {
		sink.Cancel()
		return err
	}

	if _, err := sink.Write(data); err != nil {
		sink.Cancel()
		return err
	}
	return sink.Close()
}

func (s *snapshot) Release() {}
