package config

// StorageStore 数据存储配置持久化
type StorageStore struct {
	*Manager
}

func NewStorageStore(m *Manager) *StorageStore {
	return &StorageStore{Manager: m}
}

type StorageSettings struct {
	OutputDir   string `json:"outputDir"`
	FilePrefix  string `json:"filePrefix"`
	AutoSave    bool   `json:"autoSave"`
	Format      string `json:"format"` // "csv" | "tsv"
	MaxFileSize int64  `json:"maxFileSize"`
}

func (s *StorageStore) LoadSettings() (*StorageSettings, error) {
	var settings StorageSettings
	if err := s.Load("storage", &settings); err != nil {
		return &StorageSettings{Format: "csv"}, nil
	}
	return &settings, nil
}

func (s *StorageStore) SaveSettings(settings *StorageSettings) error {
	return s.Save("storage", settings)
}
