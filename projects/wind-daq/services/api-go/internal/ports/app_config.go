package ports

// AppConfigStore 应用配置存储接口
type AppConfigStore interface {
	LoadConfig(key string) ([]byte, error)
	SaveConfig(key string, data []byte) error
}
