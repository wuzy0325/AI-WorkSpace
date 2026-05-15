package config

// AcquisitionStore 采集配置持久化
type AcquisitionStore struct {
	*Manager
}

func NewAcquisitionStore(m *Manager) *AcquisitionStore {
	return &AcquisitionStore{Manager: m}
}

func (s *AcquisitionStore) LoadPublishRate() (float64, error) {
	var data struct {
		PublishHz float64 `json:"publishHz"`
	}
	if err := s.Load("acquisition", &data); err != nil {
		return 20, nil // 默认 20Hz
	}
	if data.PublishHz <= 0 {
		return 20, nil
	}
	return data.PublishHz, nil
}

func (s *AcquisitionStore) SavePublishRate(hz float64) error {
	return s.Save("acquisition", map[string]float64{"publishHz": hz})
}
