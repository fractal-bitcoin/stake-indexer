package rdb

import (
	"context"
	"fmt"

	redis "github.com/go-redis/redis/v8"
	"github.com/spf13/viper"
)

var (
	RdbBalanceClient redis.UniversalClient
	RdbUtxoClient    redis.UniversalClient
	ctx              = context.Background()
)

func InitAll(balanceConfigFile, utxoConfigFile string) error {
	balanceClient, err := Init(balanceConfigFile)
	if err != nil {
		return fmt.Errorf("init balance redis failed: %w", err)
	}
	utxoClient, err := Init(utxoConfigFile)
	if err != nil {
		return fmt.Errorf("init utxo redis failed: %w", err)
	}
	RdbBalanceClient = balanceClient
	RdbUtxoClient = utxoClient
	return nil
}

func InitClient(filename string) (*redis.Client, error) {
	vp := viper.New()
	vp.SetConfigFile(filename)
	if err := vp.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read redis config failed: %w", err)
	}

	addr := vp.GetString("addr")
	password := vp.GetString("password")
	database := vp.GetInt("database")
	dialTimeout := vp.GetDuration("dialTimeout")
	readTimeout := vp.GetDuration("readTimeout")
	writeTimeout := vp.GetDuration("writeTimeout")
	idleTimeout := vp.GetDuration("idleTimeout")
	idleCheckFrequency := vp.GetDuration("idleCheckFrequency")
	poolSize := vp.GetInt("poolSize")
	rds := redis.NewClient(&redis.Options{
		Addr:               addr,
		Password:           password,
		DB:                 database,
		DialTimeout:        dialTimeout,
		ReadTimeout:        readTimeout,
		WriteTimeout:       writeTimeout,
		PoolSize:           poolSize,
		IdleTimeout:        idleTimeout,
		IdleCheckFrequency: idleCheckFrequency,
	})
	return rds, nil
}

func Init(filename string) (redis.UniversalClient, error) {
	vp := viper.New()
	vp.SetConfigFile(filename)
	if err := vp.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read redis config failed: %w", err)
	}

	addrs := vp.GetStringSlice("addrs")
	password := vp.GetString("password")
	database := vp.GetInt("database")
	dialTimeout := vp.GetDuration("dialTimeout")
	readTimeout := vp.GetDuration("readTimeout")
	writeTimeout := vp.GetDuration("writeTimeout")
	idleTimeout := vp.GetDuration("idleTimeout")
	idleCheckFrequency := vp.GetDuration("idleCheckFrequency")
	poolSize := vp.GetInt("poolSize")
	useCluster := vp.GetBool("useCluster")

	if useCluster {
		rds := redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:              addrs,
			Password:           password,
			DialTimeout:        dialTimeout,
			ReadTimeout:        readTimeout,
			WriteTimeout:       writeTimeout,
			PoolSize:           poolSize,
			IdleTimeout:        idleTimeout,
			IdleCheckFrequency: idleCheckFrequency,
		})
		return rds, nil
	}

	rds := redis.NewUniversalClient(&redis.UniversalOptions{
		Addrs:              addrs,
		Password:           password,
		DB:                 database,
		DialTimeout:        dialTimeout,
		ReadTimeout:        readTimeout,
		WriteTimeout:       writeTimeout,
		PoolSize:           poolSize,
		IdleTimeout:        idleTimeout,
		IdleCheckFrequency: idleCheckFrequency,
	})
	return rds, nil
}
