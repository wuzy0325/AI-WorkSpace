package main

import (
	"fmt"

	"daq-mvp/internal/usecase"

	"github.com/lxn/walk"
)

// MetricsDisplay shows acquisition counters and per-channel values.
type MetricsDisplay struct {
	batchLabel  *walk.Label
	sampleLabel *walk.Label
	rateLabel   *walk.Label
	seqLabel    *walk.Label
	chLabels    [4]*walk.Label
}

func NewMetricsDisplay(parent walk.Container) (*MetricsDisplay, error) {
	m := &MetricsDisplay{}

	composite, err := walk.NewComposite(parent)
	if err != nil {
		return nil, err
	}
	composite.SetLayout(walk.NewHBoxLayout())
	composite.Layout().SetMargins(walk.Margins{HNear: 8, VNear: 4, HFar: 8, VFar: 4})

	makeLabel := func(text string) (*walk.Label, error) {
		lbl, err := walk.NewLabel(composite)
		if err != nil {
			return nil, err
		}
		lbl.SetText(text)
		return lbl, nil
	}

	m.batchLabel, err = makeLabel("Batches: 0")
	if err != nil {
		return nil, err
	}
	m.sampleLabel, err = makeLabel("Samples: 0")
	if err != nil {
		return nil, err
	}
	m.rateLabel, err = makeLabel("Rate: 1000 Hz")
	if err != nil {
		return nil, err
	}
	m.seqLabel, err = makeLabel("Seq: 0")
	if err != nil {
		return nil, err
	}

	for i := range m.chLabels {
		m.chLabels[i], err = makeLabel(fmt.Sprintf("CH%d: 0.0000", i))
		if err != nil {
			return nil, err
		}
	}

	return m, nil
}

func (m *MetricsDisplay) Update(st usecase.Status) {
	if m.batchLabel != nil {
		m.batchLabel.SetText(fmt.Sprintf("Batches: %d", st.BatchCount))
	}
	if m.sampleLabel != nil {
		m.sampleLabel.SetText(fmt.Sprintf("Samples: %d", st.SampleCount))
	}
	if m.rateLabel != nil {
		m.rateLabel.SetText(fmt.Sprintf("Rate: %.0f Hz", st.SampleRateHz))
	}
	if m.seqLabel != nil {
		m.seqLabel.SetText(fmt.Sprintf("Seq: %d", st.BatchCount))
	}
	lv := st.LatestValues
	for i := range m.chLabels {
		if m.chLabels[i] != nil && i < len(lv) {
			m.chLabels[i].SetText(fmt.Sprintf("CH%d: %+.4f", i, lv[i]))
		}
	}
}
