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
	"strings"
	"fmt"

	"github.com/pingcap/log"
	timodel "github.com/pingcap/tidb/parser/model"
	timysql "github.com/pingcap/tidb/parser/mysql"
	"github.com/pingcap/tiflow/cdc/model"
//	"github.com/pingcap/tiflow/pkg/quotes"
//	pmysql "github.com/pingcap/tiflow/pkg/sink/mysql"
	"go.uber.org/zap"
)



var type2Str = map[byte]string{
	timysql.TypeBit:         "bit",
	timysql.TypeBlob:        "text",
	timysql.TypeDate:        "date",
	timysql.TypeDatetime:    "datetime",
	timysql.TypeUnspecified: "unspecified",
	timysql.TypeNewDecimal:  "decimal",
	timysql.TypeDouble:      "double",
	timysql.TypeEnum:        "enum",
	timysql.TypeFloat:       "float",
	timysql.TypeGeometry:    "geometry",
	timysql.TypeInt24:       "mediumint",
	timysql.TypeJSON:        "json",
	timysql.TypeLong:        "int",
	timysql.TypeLonglong:    "bigint",
	timysql.TypeLongBlob:    "longtext",
	timysql.TypeMediumBlob:  "mediumtext",
	timysql.TypeNull:        "null",
	timysql.TypeSet:         "set",
	timysql.TypeShort:       "smallint",
	timysql.TypeString:      "char",
	timysql.TypeDuration:    "time",
	timysql.TypeTimestamp:   "timestamp",
	timysql.TypeTiny:        "tinyint",
	timysql.TypeTinyBlob:    "tinytext",
	timysql.TypeVarchar:     "varchar",
	timysql.TypeVarString:   "var_string",
	timysql.TypeYear:        "year",
}

// TypeStr converts tp to a string.
func TypeStr(tp byte) (r string) {
	return type2Str[tp]
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

/*
ddl="{
	"StartTs":440551752425537545
      , "CommitTs":440551752438644738
      , "Query":"DROP TABLE `test`.`test04`"
      , "TableInfo":{"id":86, "name":{"O":"test04", "L":"test04"}
      , "charset":"utf8mb4"
      , "collate":"utf8mb4_bin"
      , "cols":[{"id":1, "name":{"O":"col01", "L":"col01"}, "offset":0, "origin_default":null, "origin_default_bit":null, "default":null, "default_bit":null, 
"default_is_expr":false, "generated_expr_string":"", "generated_stored":false, "dependences":null
          , "type":{"Tp":3, "Flag":4099, "Flen":11, "Decimal":0, "Charset":"binary", "Collate":"binary", "Elems":null, "ElemsIsBinaryLit":null, "Array":false}, 
"state":5, 
"comment":"", 
"hidden":false, 
"change_state_info":null, 
"version":2}, 
{"id":2, 
"name":{"O":"col02", 
"L":"col02"}, 
"offset":1, 
"origin_default":null, 
"origin_default_bit":null, 
"default":null, 
"default_bit":null, 
"default_is_expr":false, 
"generated_expr_string":"", 
"generated_stored":false, 
"dependences":null, 
"type":{"Tp":3, 
"Flag":0, 
"Flen":11, 
"Decimal":0, 
"Charset":"binary", 
"Collate":"binary", 
"Elems":null, 
"ElemsIsBinaryLit":null, 
"Array":false}, 
"state":5, 
"comment":"", 
"hidden":false, 
"change_state_info":null, 
"version":2}], 
"index_info":null, 
"constraint_info":null, 
"fk_info":null, 
"state":0, 
"pk_is_handle":true, 
"is_common_handle":false, 
"common_handle_version":0, 
"comment":"", 
"auto_inc_id":0, 
"auto_id_cache":0, 
"auto_rand_id":0, 
"max_col_id":2, 
"max_idx_id":0, 
"max_fk_id":0, 
"max_cst_id":0, 
"update_timestamp":440540393777659910, 
"ShardRowIDBits":0, 
"max_shard_row_id_bits":0, 
"auto_random_bits":0, 
"auto_random_range_bits":0, 
"pre_split_regions":0, 
"partition":null, 
"compression":"", 
"view":null, 
"sequence":null, 
"Lock":null, 
"version":5, 
"tiflash_replica":null, 
"is_columnar":false, 
"temp_table_type":0, 
"cache_table_status":0, 
"policy_ref_info":null, 
"stats_options":null, 
"exchange_partition_info":null, 
"ttl_info":null, 
"SchemaID":2, 
"TableName":{"db-name":"test", 
"tbl-name":"test04", 
"tbl-id":86, 
"is-partition":false}, 
"Version":440551752438644738, 
"RowColumnsOffset":{"1":0, 
"2":1}, 
"ColumnsFlag":{"1":11, 
"2":65}, 
"HandleIndexID":-1, 
"IndexColumnsOffset":[[0]]}, 
"PreTableInfo":{"id":86, 
"name":{"O":"test04", 
"L":"test04"}, 
"charset":"utf8mb4", 
"collate":"utf8mb4_bin", 
"cols":[{"id":1, 
"name":{"O":"col01", 
"L":"col01"}, 
"offset":0, 
"origin_default":null, 
"origin_default_bit":null, 
"default":null, 
"default_bit":null, 
"default_is_expr":false, 
"generated_expr_string":"", 
"generated_stored":false, 
"dependences":null, 
"type":{"Tp":3, 
"Flag":4099, 
"Flen":11, 
"Decimal":0, 
"Charset":"binary", 
"Collate":"binary", 
"Elems":null, 
"ElemsIsBinaryLit":null, 
"Array":false}, 
"state":5, 
"comment":"", 
"hidden":false, 
"change_state_info":null, 
"version":2}, 
{"id":2, 
"name":{"O":"col02", 
"L":"col02"}, 
"offset":1, 
"origin_default":null, 
"origin_default_bit":null, 
"default":null, 
"default_bit":null, 
"default_is_expr":false, 
"generated_expr_string":"", 
"generated_stored":false, 
"dependences":null, 
"type":{"Tp":3, 
"Flag":0, 
"Flen":11, 
"Decimal":0, 
"Charset":"binary", 
"Collate":"binary", 
"Elems":null, 
"ElemsIsBinaryLit":null, 
"Array":false}, 
"state":5, 
"comment":"", 
"hidden":false, 
"change_state_info":null, 
"version":2}], 
"index_info":null, 
"constraint_info":null, 
"fk_info":null, 
"state":5, 
"pk_is_handle":true, 
"is_common_handle":false, 
"common_handle_version":0, 
"comment":"", 
"auto_inc_id":0, 
"auto_id_cache":0, 
"auto_rand_id":0, 
"max_col_id":2, 
"max_idx_id":0, 
"max_fk_id":0, 
"max_cst_id":0, 
"update_timestamp":440540393777659910, 
"ShardRowIDBits":0, 
"max_shard_row_id_bits":0, 
"auto_random_bits":0, 
"auto_random_range_bits":0, 
"pre_split_regions":0, 
"partition":null, 
"compression":"", 
"view":null, 
"sequence":null, 
"Lock":null, 
"version":5, 
"tiflash_replica":null, 
"is_columnar":false, 
"temp_table_type":0, 
"cache_table_status":0, 
"policy_ref_info":null, 
"stats_options":null, 
"exchange_partition_info":null, 
"ttl_info":null, 
"SchemaID":2, 
"TableName":{"db-name":"test", 
"tbl-name":"test04", 
"tbl-id":86, 
"is-partition":false}, 
"Version":440551749096308739, 
"RowColumnsOffset":{"1":0, 
"2":1}, 
"ColumnsFlag":{"1":11, 
"2":65}, 
"HandleIndexID":-1, 
"IndexColumnsOffset":[[0]]}, 
"Type":4, 
"Done":false}"
*/
func (m *oracleDDLSink)makeDDLQuery(ddl *model.DDLEvent) (string, error) {
    // According the Type to determine the action of the ddl. Please refer to 
    // https://github.com/pingcap/parser/blob/master/model/ddl.go for all the ActionType.
    switch ddl.Type {
        case timodel.ActionCreateTable:
            log.Info("Starting to create the create statement. ")
	    return m.prepareCreateDDL(ddl)
        case timodel.ActionDropTable:
            log.Info("TODO: Starting to prepare drop table statement. ")
	    return m.prepareDropDDL(ddl)
        case timodel.ActionAddColumn:
            log.Info("TODO: Starting to prepare column addition statement. ")
        case timodel.ActionDropColumn:
            log.Info("TODO: Starting to prepare column drop statement. ")
        case timodel.ActionAddForeignKey:
            log.Info("TODO: Starting to prepare foreign key addition statement. ")
        case timodel.ActionDropForeignKey:
            log.Info("TODO: Starting to prepare foreign key drop statement. ")
        case timodel.ActionTruncateTable:
            log.Info("TODO: Starting to prepare table truncate statement. ")
        case timodel.ActionModifyColumn:
            log.Info("TODO: Starting to prepare column modification statement. ")
        case timodel.ActionRenameTable:
            log.Info("TODO: Starting to prepare table rename statement. ")
        case timodel.ActionAddTablePartition:
            log.Info("TODO: Starting to prepare table partition adition statement. ")
        case timodel.ActionDropTablePartition:
            log.Info("TODO: Starting to prepare table parition drop statement. ")
        case timodel.ActionAddPrimaryKey:
            log.Info("TODO: Starting to prepare primary key addition statement. ")
        case timodel.ActionDropPrimaryKey:
            log.Info("TODO: Starting to prepare primary key drop statement. ")
        case timodel.ActionAddColumns:
            log.Info("TODO: Starting to prepare column addition statement. ")
        case timodel.ActionDropColumns:
            log.Info("TODO: Starting to prepare column drop statement. ")
        case timodel.ActionRenameTables:
            log.Info("TODO: Starting to prepare table rename statement. ")
        default:
            log.Info("Unsupported ddl action type<%s>.", zap.String("action type", string(ddl.Type) ))
    }
    return "", nil
}

/*
01. Prepare the whole structre of the creat table
02. Prepare Primary key statement
03. Prepare foreign key statement
04. Prepare column statement
*/
func (m *oracleDDLSink)prepareCreateDDL(ddl *model.DDLEvent) (string, error) {
    log.Info(fmt.Sprintf("Starting to prepare the create ddl: <%#v> ", ddl))
    // https://github.com/pingcap/tiflow/blob/master/cdc/model/sink.go  -> IsPrimaryKey
    tableInfo := ddl.TableInfo
    log.Info(fmt.Sprintf("table info: <%#v>", tableInfo))
    log.Info(fmt.Sprintf("table info: <%#v>", tableInfo.ColumnsFlag))
    handleColID, rowColFieldsTps, rowColInfos := tableInfo.GetRowColInfos()
    log.Info(fmt.Sprintf("table info: <%#v>", handleColID))
    log.Info(fmt.Sprintf("row col field tps: <%#v>", rowColFieldsTps))
    log.Info(fmt.Sprintf("row col info: <%#v>", rowColInfos))

    var pkCols []string
    var cols []string
    for _, column := range tableInfo.Columns {
        log.Info("Columns info: ", zap.String("columns", fmt.Sprintf("%#v", column)))
	colFlag := tableInfo.ColumnsFlag[column.ID]
	log.Info("the col flag is:", zap.String("col", fmt.Sprintf("%#v", colFlag) ))
	if colFlag.IsPrimaryKey() {
	    log.Info("This is primary key")
            pkCols = append(pkCols, column.Name.O)
	    log.Info("PK array", zap.String("col", fmt.Sprintf("%#v", pkCols)))
	}

        var colEle []string
        colEle = append(colEle, column.Name.O)

	// Field type
	fieldType := TypeStr(column.FieldType.GetType())
	log.Info("Field type", zap.String("field type", fieldType))
        colEle = append(colEle, fieldType)

       if colFlag.IsNullable() == false {
           colEle = append(colEle, "not null")
       }
       cols = append(cols, strings.Join(colEle, " "))

    }

    pkStmt := fmt.Sprintf("CONSTRAINT pk_%s_%s PRIMARY KEY(%s)", tableInfo.TableName.Schema, tableInfo.TableName.Table, strings.Join(pkCols, ", "))
    log.Info("creation preparation", zap.String("PK", pkStmt ))

    colStmt := strings.Join(cols, ", ")
    log.Info("columns: ", zap.String("column stmt", colStmt))

    return fmt.Sprintf("create table %s.%s (%s, %s)", tableInfo.TableName.Schema, tableInfo.TableName.Table, colStmt, pkStmt), nil
}

func (m *oracleDDLSink)prepareDropDDL(ddl *model.DDLEvent) (string, error) {
    tableInfo := ddl.TableInfo
    return fmt.Sprintf("DROP TABLE %s.%s", tableInfo.TableName.Schema, tableInfo.TableName.Table), nil
}

