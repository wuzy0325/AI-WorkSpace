package ports

// ==================== 配置持久化接口 ====================
// 配置仓库抽象接口，支持从文件/数据库加载和保存配置

// ConfigRepo 配置仓库接口
// 将配置数据持久化到存储介质（文件系统、数据库等）
type ConfigRepo interface {
	// Load 加载指定名称的配置到v指向的对象
	// 参数: name 配置名称,如"device-profiles"
	// 参数: v 目标对象指针,需为具体类型的指针
	Load(name string, v interface{}) error
	// Save 保存对象v到指定名称的配置存储
	// 参数: name 配置名称
	// 参数: v 要保存的对象
	Save(name string, v interface{}) error
	// Dir 返回配置存储目录路径
	Dir() string
}
