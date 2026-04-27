package ports

type PublishRateStore interface {
	LoadPublishRate() (float64, error)
	SavePublishRate(hz float64) error
}
