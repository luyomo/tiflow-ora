// Copyright 2022 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package oracle

import (
	"context"
	"database/sql"
	"net/url"
	"time"
	"fmt"

	"github.com/pingcap/errors"
	"github.com/pingcap/failpoint"
	"github.com/pingcap/log"
	"github.com/pingcap/tiflow/cdc/contextutil"
	"github.com/pingcap/tiflow/cdc/model"
	"github.com/pingcap/tiflow/cdc/sinkv2/ddlsink"
	"github.com/pingcap/tiflow/cdc/sinkv2/metrics"
	"github.com/pingcap/tiflow/pkg/config"
	cerror "github.com/pingcap/tiflow/pkg/errors"
	"github.com/pingcap/tiflow/pkg/errorutil"
//	"github.com/pingcap/tiflow/pkg/quotes"
	"github.com/pingcap/tiflow/pkg/retry"
	"github.com/pingcap/tiflow/pkg/sink"
//	pmysql "github.com/pingcap/tiflow/pkg/sink/mysql"
	poracle "github.com/pingcap/tiflow/pkg/sink/oracle"
	"go.uber.org/zap"
)

const (
	defaultDDLMaxRetry uint64 = 20

	// networkDriftDuration is used to construct a context timeout for database operations.
	networkDriftDuration = 5 * time.Second
)

// Assert DDLEventSink implementation
var _ ddlsink.DDLEventSink = (*oracleDDLSink)(nil)

type oracleDDLSink struct {
	// id indicates which processor (changefeed) this sink belongs to.
	id model.ChangeFeedID
	// db is the database connection.
	db  *sql.DB
	cfg *poracle.Config
	// statistics is the statistics of this sink.
	// We use it to record the DDL count.
	statistics *metrics.Statistics
}

// NewOracleDDLSink creates a new mysqlDDLSink.
func NewOracleDDLSink(
	ctx context.Context,
	sinkURI *url.URL,
	replicaConfig *config.ReplicaConfig,
	dbConnFactory poracle.Factory,
) (*oracleDDLSink, error) {
	// To remove: Get the change feed it
	changefeedID := contextutil.ChangefeedIDFromCtx(ctx)

	// To remove: Set the config file from uri and db
	cfg := poracle.NewConfig()
	err := cfg.Apply(ctx, changefeedID, sinkURI, replicaConfig)
	if err != nil {
		return nil, err
	}

	// To remove: Generate the connection string
	dsnStr, err := poracle.GenerateDSN(ctx, sinkURI, cfg, dbConnFactory)
	if err != nil {
		return nil, err
	}
	log.Info("oracle connection string in the ddl", zap.String("oracle", dsnStr))

	// To remove: Set the db connection
	db, err := dbConnFactory(ctx, dsnStr)
	if err != nil {
		return nil, err
	}

	// To remove: set the oracle ddl sink object
	m := &oracleDDLSink{
		id:         changefeedID,
		db:         db,
		cfg:        cfg,
		statistics: metrics.NewStatistics(ctx, sink.TxnSink),
	}

	log.Info("Oracle DDL sink is created",
		zap.String("namespace", m.id.Namespace),
		zap.String("changefeed", m.id.ID))
	return m, nil
}

func (m *oracleDDLSink) WriteDDLEvent(ctx context.Context, ddl *model.DDLEvent) error {
        log.Info("Starting oracle ddl")
	err := m.execDDLWithMaxRetries(ctx, ddl)
	return errors.Trace(err)
}

func (m *oracleDDLSink) execDDLWithMaxRetries(ctx context.Context, ddl *model.DDLEvent) error {
	return retry.Do(ctx, func() error {
		err := m.statistics.RecordDDLExecution(func() error { return m.execDDL(ctx, ddl) })
		if err != nil {
			if errorutil.IsIgnorableMySQLDDLError(err) {
				// NOTE: don't change the log, some tests depend on it.
				log.Info("Execute DDL failed, but error can be ignored",
					zap.Uint64("startTs", ddl.StartTs), zap.String("ddl", ddl.Query),
					zap.String("namespace", m.id.Namespace),
					zap.String("changefeed", m.id.ID),
					zap.Error(err))
				// If the error is ignorable, we will direly ignore the error.
				return nil
			}
			log.Warn("Execute DDL with error, retry later",
				zap.Uint64("startTs", ddl.StartTs), zap.String("ddl", ddl.Query),
				zap.String("namespace", m.id.Namespace),
				zap.String("changefeed", m.id.ID),
				zap.Error(err))
			return err
		}
		return nil
	}, retry.WithBackoffBaseDelay(poracle.BackoffBaseDelay.Milliseconds()),
		retry.WithBackoffMaxDelay(poracle.BackoffMaxDelay.Milliseconds()),
		retry.WithMaxTries(defaultDDLMaxRetry),
		retry.WithIsRetryableErr(cerror.IsRetryableError))
}

func (m *oracleDDLSink) execDDL(pctx context.Context, ddl *model.DDLEvent) error {
	writeTimeout, _ := time.ParseDuration(m.cfg.WriteTimeout)
	writeTimeout += networkDriftDuration
	ctx, cancelFunc := context.WithTimeout(pctx, writeTimeout)
	defer cancelFunc()

	// Not required any more
	// shouldSwitchDB := needSwitchDB(ddl)

	failpoint.Inject("OracleSinkExecDDLDelay", func() {
		select {
		case <-ctx.Done():
			failpoint.Return(ctx.Err())
		case <-time.After(time.Hour):
		}
		failpoint.Return(nil)
	})

	start := time.Now()
	log.Info("Start exec DDL", zap.Any("DDL", ddl), zap.String("namespace", m.id.Namespace),
		zap.String("changefeed", m.id.ID))
	ddlQuery, err := m.makeDDLQuery(ddl)
	if err != nil {
		return err
	}

        if ddlQuery == "" {
            return nil
        }

	log.Info(fmt.Sprintf("The ddl is <%#v>", ddlQuery) )

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	if _, err = tx.ExecContext(ctx, ddlQuery); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			log.Error("Failed to rollback", zap.String("sql", ddlQuery),
				zap.String("namespace", m.id.Namespace),
				zap.String("changefeed", m.id.ID), zap.Error(err))
		}
		return err
	}

	if err = tx.Commit(); err != nil {
		log.Error("Failed to exec DDL", zap.String("sql", ddlQuery),
			zap.Duration("duration", time.Since(start)),
			zap.String("namespace", m.id.Namespace),
			zap.String("changefeed", m.id.ID), zap.Error(err))
		return cerror.WrapError(cerror.ErrMySQLTxnError, err)
	}

	log.Info("Exec DDL succeeded", zap.String("sql", ddlQuery),
		zap.Duration("duration", time.Since(start)),
		zap.String("namespace", m.id.Namespace),
		zap.String("changefeed", m.id.ID))
	return nil
}

func (m *oracleDDLSink) WriteCheckpointTs(_ context.Context, _ uint64, _ []*model.TableInfo) error {
	// Only for RowSink for now.
	return nil
}

// Close closes the database connection.
func (m *oracleDDLSink) Close() {
	if m.statistics != nil {
		m.statistics.Close()
	}
	if m.db != nil {
		if err := m.db.Close(); err != nil {
			log.Warn("MySQL ddl sink close db wit error",
				zap.String("namespace", m.id.Namespace),
				zap.String("changefeed", m.id.ID),
				zap.Error(err))
		}
	}
}
