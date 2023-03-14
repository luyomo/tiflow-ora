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
	"strings"
	"fmt"

	"github.com/pingcap/errors"
	"github.com/pingcap/failpoint"
	"github.com/pingcap/log"
	timodel "github.com/pingcap/tidb/parser/model"
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
	changefeedID := contextutil.ChangefeedIDFromCtx(ctx)
	cfg := poracle.NewConfig()
	err := cfg.Apply(ctx, changefeedID, sinkURI, replicaConfig)
	if err != nil {
		return nil, err
	}

	dsnStr, err := poracle.GenerateDSN(ctx, sinkURI, cfg, dbConnFactory)
	if err != nil {
		return nil, err
	}

	db, err := dbConnFactory(ctx, dsnStr)
	if err != nil {
		return nil, err
	}

	m := &oracleDDLSink{
		id:         changefeedID,
		db:         db,
		cfg:        cfg,
		statistics: metrics.NewStatistics(ctx, sink.TxnSink),
	}

	log.Info("MySQL DDL sink is created",
		zap.String("namespace", m.id.Namespace),
		zap.String("changefeed", m.id.ID))
	return m, nil
}

func (m *oracleDDLSink) WriteDDLEvent(ctx context.Context, ddl *model.DDLEvent) error {
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

	shouldSwitchDB := needSwitchDB(ddl)

	failpoint.Inject("MySQLSinkExecDDLDelay", func() {
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
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	log.Info("execDDL 001", zap.String("value", fmt.Sprintf("%#v", shouldSwitchDB) ))

//	if shouldSwitchDB {
//		log.Info("execDDL 001-02")
//		_, err = tx.ExecContext(ctx, "USE "+quotes.QuoteName(ddl.TableInfo.TableName.Schema)+";")
//		if err != nil {
//			if rbErr := tx.Rollback(); rbErr != nil {
//				log.Error("Failed to rollback", zap.String("namespace", m.id.Namespace),
//					zap.String("changefeed", m.id.ID), zap.Error(err))
//			}
//			return err
//		}
//	}
//	log.Info("execDDL 002")

	query := strings.ReplaceAll(ddl.Query, "`", "")
	//if _, err = tx.ExecContext(ctx, ddl.Query); err != nil {
	if _, err = tx.ExecContext(ctx, query); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			log.Error("Failed to rollback", zap.String("sql", ddl.Query),
				zap.String("namespace", m.id.Namespace),
				zap.String("changefeed", m.id.ID), zap.Error(err))
		}
		return err
	}
	log.Info("execDDL 003")

	if err = tx.Commit(); err != nil {
		log.Error("Failed to exec DDL", zap.String("sql", ddl.Query),
			zap.Duration("duration", time.Since(start)),
			zap.String("namespace", m.id.Namespace),
			zap.String("changefeed", m.id.ID), zap.Error(err))
		return cerror.WrapError(cerror.ErrMySQLTxnError, err)
	}

	log.Info("Exec DDL succeeded", zap.String("sql", ddl.Query),
		zap.Duration("duration", time.Since(start)),
		zap.String("namespace", m.id.Namespace),
		zap.String("changefeed", m.id.ID))
	return nil
}

/*
Sample data: 
{
	"StartTs":440079336227471363
      , "CommitTs":440079336227471370
      , "Query":"CREATE TABLE `test01` (`col01` INT PRIMARY KEY,`col02` INT)"
      , "TableInfo":{
	      "id":80
            , "name":{"O":"test01","L":"test01"}
	    , "charset":"utf8mb4"
	    , "collate":"utf8mb4_bin"
	    , "cols":[{"id":1,"name":{"O":"col01","L":"col01"},"offset":0,"origin_default":null,"origin_default_bit":null,"default":null,"default_bit":null,"default_is_expr":false,"generated_expr_string":"","generated_stored":false,"dependences":null,"type":{"Tp":3,"Flag":4099,"Flen":11,"Decimal":0,"Charset":"binary","Collate":"binary","Elems":null,"ElemsIsBinaryLit":null,"Array":false},"state":5,"comment":"","hidden":false,"change_state_info":null,"version":2},{"id":2,"name":{"O":"col02","L":"col02"},"offset":1,"origin_default":null,"origin_default_bit":null,"default":null,"default_bit":null,"default_is_expr":false,"generated_expr_string":"","generated_stored":false,"dependences":null,"type":{"Tp":3,"Flag":0,"Flen":11,"Decimal":0,"Charset":"binary","Collate":"binary","Elems":null,"ElemsIsBinaryLit":null,"Array":false},"state":5,"comment":"","hidden":false,"change_state_info":null,"version":2}]
	    , "index_info":null
	    , "constraint_info":null
	    , "fk_info":null
	    , "state":5
	    , "pk_is_handle":true
	    , "is_common_handle":false
	    , "common_handle_version":0
	    , "comment":""
	    , "auto_inc_id":0
	    , "auto_id_cache":0
	    , "auto_rand_id":0
	    , "max_col_id":2
	    , "max_idx_id":0
	    , "max_fk_id":0
	    , "max_cst_id":0
	    , "update_timestamp":440079336227471363
	    , "ShardRowIDBits":0
	    , "max_shard_row_id_bits":0
	    , "auto_random_bits":0
	    , "auto_random_range_bits":0
	    , "pre_split_regions":0
	    , "partition":null
	    , "compression":""
	    , "view":null
	    , "sequence":null
	    , "Lock":null
	    , "version":5
	    , "tiflash_replica":null
	    , "is_columnar":false
	    , "temp_table_type":0
	    , "cache_table_status":0
	    , "policy_ref_info":null
	    , "stats_options":null
	    , "exchange_partition_info":null
	    , "ttl_info":null
	    , "SchemaID":2
	    , "TableName":{
		    "db-name":"test"
		  , "tbl-name":"test01"
		  , "tbl-id":80
		  , "is-partition":false}
            , "Version":440079336227471370
	    , "RowColumnsOffset":{"1":0,"2":1}
	    , "ColumnsFlag":{"1":11,"2":65}
	    , "HandleIndexID":-1
	    , "IndexColumnsOffset":[[0]]}
      , "PreTableInfo":null
      , "Type":3
      , "Done":false}
*/
func makeDDL(ddl *model.DDLEvent) (string, error) {
    return "", nil
}

func needSwitchDB(ddl *model.DDLEvent) bool {
	if len(ddl.TableInfo.TableName.Schema) == 0 {
		return false
	}
	if ddl.Type == timodel.ActionCreateSchema || ddl.Type == timodel.ActionDropSchema {
		return false
	}
	return true
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
