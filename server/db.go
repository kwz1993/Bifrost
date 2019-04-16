/*
Copyright [2018] [jc3wish]

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package server

import (
	"sync"

	"github.com/jc3wish/Bifrost/Bristol/mysql"
	"log"
	"github.com/jc3wish/Bifrost/server/count"
	"github.com/jc3wish/Bifrost/server/warning"
	"time"
	"strings"
	"strconv"
)

var DbLock sync.Mutex

var DbList map[string]*db

func init() {
	DbList = make(map[string]*db, 0)
}

func AddNewDB(Name string, ConnectUri string, binlogFileName string, binlogPostion uint32, serverId uint32,maxFileName string,maxPosition uint32,AddTime int64) *db {
	var r bool = false
	DbLock.Lock()
	if _, ok := DbList[Name]; !ok {
		DbList[Name] = NewDb(Name, ConnectUri, binlogFileName, binlogPostion, serverId,maxFileName,maxPosition,AddTime)
		r = true
	}
	count.SetDB(Name)
	DbLock.Unlock()
	log.Println("Add db Info:",Name,ConnectUri,binlogFileName,binlogPostion,serverId,maxFileName,maxPosition)
	if r == true {
		return DbList[Name]
	} else {
		return nil
	}
}

func GetDBObj(Name string) *db{
	if _,ok := DbList[Name];!ok{
		return nil
	}
	return DbList[Name]
}


func DelDB(Name string) bool {
	DbLock.Lock()
	defer DbLock.Unlock()
	DBPositionBinlogKey := getDBBinlogkey(DbList[Name])
	if _, ok := DbList[Name]; ok {
		if DbList[Name].ConnStatus == "close" {
			for _,c := range  DbList[Name].channelMap{
				count.DelChannel(Name,c.Name)
			}
			delete(DbList, Name)
			count.DelDB(Name)
			log.Println("delete db:",Name)
		} else {
			return false
		}
	}
	// 删除binlog 信息
	delBinlogPosition(DBPositionBinlogKey)
	return true
}

type db struct {
	sync.RWMutex
	Name               		string `json:"Name"`
	ConnectUri         		string `json:"ConnectUri"`
	ConnStatus         		string `json:"ConnStatus"` //close,stop,starting,running
	ConnErr            		string `json:"ConnErr"`
	channelMap         		map[int]*Channel `json:"ChannelMap"`
	LastChannelID      		int	`json:"LastChannelID"`
	tableMap           		map[string]*Table `json:"TableMap"`
	binlogDump         		*mysql.BinlogDump
	binlogDumpFileName 		string `json:"BinlogDumpFileName"`
	binlogDumpPosition 		uint32 `json:"BinlogDumpPosition"`
	replicateDoDb      		map[string]uint8 `json:"ReplicateDoDb"`
	serverId           		uint32 `json:"ServerId"`
	killStatus 				int
	maxBinlogDumpFileName 	string `json:"MaxBinlogDumpFileName"`
	maxBinlogDumpPosition 	uint32 `json:"MaxBinlogDumpPosition"`
	AddTime					int64
	DBBinlogKey				[]byte `json:"-"`  // 保存 binlog到levelDB 的key
}

type DbListStruct struct {
	Name               		string
	ConnectUri         		string
	ConnStatus         		string //close,stop,starting,running
	ConnErr            		string
	ChannelCount       		int
	LastChannelID      		int
	TableCount         		int
	BinlogDumpFileName 		string
	BinlogDumpPosition 		uint32
	MaxBinlogDumpFileName 	string
	MaxBinlogDumpPosition 	uint32
	ReplicateDoDb      		map[string]uint8
	ServerId           		uint32
	AddTime					int64
}

func GetListDb() map[string]DbListStruct {
	var dbListMap map[string]DbListStruct
	dbListMap = make(map[string]DbListStruct,0)
	DbLock.Lock()
	defer DbLock.Unlock()
	for k,v := range DbList{
		dbListMap[k] = DbListStruct{
			Name:					v.Name,
			ConnectUri:				v.ConnectUri,
			ConnStatus:				v.ConnStatus,
			ConnErr:				v.ConnErr,
			ChannelCount:			len(v.channelMap),
			LastChannelID:			v.LastChannelID,
			TableCount:				len(v.tableMap),
			BinlogDumpFileName:		v.binlogDumpFileName,
			BinlogDumpPosition:		v.binlogDumpPosition,
			MaxBinlogDumpFileName:	v.maxBinlogDumpFileName,
			MaxBinlogDumpPosition:	v.maxBinlogDumpPosition,
			ReplicateDoDb:			v.replicateDoDb,
			ServerId:				v.serverId,
			AddTime:				v.AddTime,
		}
	}
	return dbListMap
}


func NewDb(Name string, ConnectUri string, binlogFileName string, binlogPostion uint32, serverId uint32,maxFileName string,maxPosition uint32,AddTime int64) *db {
	return &db{
		Name:               	Name,
		ConnectUri:         	ConnectUri,
		ConnStatus:         	"close",
		ConnErr:            	"",
		LastChannelID:			0,
		channelMap:         	make(map[int]*Channel, 0),
		tableMap:           	make(map[string]*Table, 0),
		binlogDumpFileName: 	binlogFileName,
		binlogDumpPosition: 	binlogPostion,
		maxBinlogDumpFileName:	maxFileName,
		maxBinlogDumpPosition:	maxPosition,
		binlogDump: 			&mysql.BinlogDump{
					DataSource:    	ConnectUri,
					ReplicateDoDb: 	make(map[string]uint8, 0),
					OnlyEvent:     	[]mysql.EventType{
										mysql.WRITE_ROWS_EVENTv2, mysql.UPDATE_ROWS_EVENTv2, mysql.DELETE_ROWS_EVENTv2,
										mysql.QUERY_EVENT,
										mysql.WRITE_ROWS_EVENTv1, mysql.UPDATE_ROWS_EVENTv1, mysql.DELETE_ROWS_EVENTv1,
										mysql.WRITE_ROWS_EVENTv0, mysql.UPDATE_ROWS_EVENTv0, mysql.DELETE_ROWS_EVENTv0,
									},
				},
		replicateDoDb: 			make(map[string]uint8, 0),
		serverId:      			serverId,
		killStatus:				0,
		AddTime:				AddTime,
	}
}
/*

func DelDb(Name string) error{
	DbLock.Lock()
	defer DbLock.Unlock()
	if _,ok := DbList[Name];!ok{
		return fmt.Errorf(Name+" not exsit")
	}
	if DbList[Name].ConnStatus == "close"{
		delete(DbList,Name)
		return nil
	}else{
		return fmt.Errorf(Name+" ConnStatus is not close")
	}
}
*/

func (db *db) SetServerId(serverId uint32) {
	db.serverId = serverId
}

func (db *db) SetReplicateDoDb(dbArr []string) bool {
	if db.ConnStatus == "close" || db.ConnStatus == "stop" {
		for i := 0; i < len(dbArr); i++ {
			db.replicateDoDb[dbArr[i]] = 1
		}
		db.binlogDump.ReplicateDoDb = db.replicateDoDb
		return true
	}
	return false
}

func (db *db) AddReplicateDoDb(dbName string) bool {
	db.Lock()
	defer db.Unlock()
	if _,ok:=db.replicateDoDb[dbName];!ok{
		db.replicateDoDb[dbName] = 1
	}
	return true
}

func (db *db) Start() (b bool) {
	b = false
	if db.maxBinlogDumpFileName == db.binlogDumpFileName && db.binlogDumpPosition >= db.maxBinlogDumpPosition{
		return
	}
	switch db.ConnStatus {
	case "close":
		db.ConnStatus = "starting"
		reslut := make(chan error, 1)
		db.binlogDump.CallbackFun = db.Callback

		go	db.binlogDump.StartDumpBinlog(db.binlogDumpFileName, db.binlogDumpPosition, db.serverId, reslut, db.maxBinlogDumpFileName, db.maxBinlogDumpPosition)

		go db.monitorDump(reslut)
		break
	case "stop":
		db.ConnStatus = "running"
		log.Println(db.Name+" monitor:","running")
		db.binlogDump.Start()
		break
	default:
		return
	}
	return true
}

func (db *db) Stop() bool {
	if db.ConnStatus == "running" {
		db.binlogDump.Stop()
		db.ConnStatus = "stop"
	}
	return true
}

func (db *db) Close() bool {
	db.ConnStatus = "closing"
	db.binlogDump.Close()
	return true
}

func (db *db) monitorDump(reslut chan error) (r bool) {
	var lastStatus string = ""
	for {
		select {
			case v := <-reslut:
			switch v.Error() {
			case "stop":
				db.ConnStatus = "stop"
				break
			case "running":
				db.ConnStatus = "running"
				db.ConnErr = "running"
				warning.AppendWarning(warning.WarningContent{
					Type:   warning.WARNINGNORMAL,
					DbName: db.Name,
					Body:   " connect status:running; last status:" + lastStatus,
				})
				break
			default:
				warning.AppendWarning(warning.WarningContent{
					Type:   warning.WARNINGERROR,
					DbName: db.Name,
					Body:   " connect status:" + v.Error() + "; last status:" + lastStatus,
				})
				db.ConnErr = v.Error()
				break
			}

			if v.Error() != lastStatus {
				log.Println(db.Name+" monitor:", v.Error())
			} else {
				lastStatus = v.Error()
			}

			if v.Error() == "close" {
				db.ConnStatus = "close"
				return
			}
			break
		case <- time.After(3 * time.Second):
			db.saveBinlog()
			break
		}
	}
	return true
}

func (db *db) saveBinlog(){
	FileName,Position := db.binlogDump.GetBinlog()
	if FileName == ""{
		return
	}
	db.Lock()
	//保存位点,这个位点在重启 配置文件恢复的时候
	//一个db有可能有多个channel，数据顺序不用担心，因为实际在重启的时候 会根据最小的 ToServerList 的位点进行自动替换
	db.binlogDumpFileName,db.binlogDumpPosition = FileName,Position
	if db.DBBinlogKey == nil{
		db.DBBinlogKey = getDBBinlogkey(db)
	}
	db.Unlock()
	index := strings.IndexAny(FileName, ".")

	BinlogFileNum,_ := strconv.Atoi(FileName[index+1:])
	saveBinlogPosition(db.DBBinlogKey,BinlogFileNum,db.binlogDumpPosition)
}

func (db *db) AddTable(schemaName string, tableName string, ChannelKey int,LastToServerID int) bool {
	key := schemaName + "-" + tableName
	if _, ok := db.tableMap[key]; !ok {
		db.tableMap[key] = &Table{
			Name:         	tableName,
			ChannelKey:   	ChannelKey,
			ToServerList: 	make([]*ToServer, 0),
			LastToServerID: LastToServerID,
		}
		log.Println("AddTable",db.Name,schemaName,tableName,db.channelMap[ChannelKey].Name)
		count.SetTable(db.Name,key)
	} else {
		log.Println("AddTable key:",key,"db.tableMap[key]：",db.tableMap[key])
		db.Lock()
		db.tableMap[key].ChannelKey = ChannelKey
		db.Unlock()
	}
	return true
}

func (db *db) GetTable(schemaName string, tableName string) *Table {
	key := schemaName + "-" + tableName
	if _, ok := db.tableMap[key]; !ok {
		return  nil
	} else {
		return db.tableMap[key]
	}
}

func (db *db) GetTables() map[string]*Table {
	return db.tableMap
}

func (db *db) GetTableByChannelKey(schemaName string, ChanneKey int) (TableMap map[string]*Table) {
	TableMap = make(map[string]*Table,0)
	for k,v := range db.tableMap{
		if v.ChannelKey == ChanneKey && len(v.ToServerList) > 0 {
			TableMap[k] = v
		}
	}
	return
}

func (db *db) DelTable(schemaName string, tableName string) bool {
	key := schemaName + "-" + tableName
	if _, ok := db.tableMap[key]; !ok {
		return true
	} else {
		db.Lock()
		for _,toServerInfo := range db.tableMap[key].ToServerList{
			//toServerInfo.Lock()
			if toServerInfo.Status == "running"{
				toServerInfo.Status = "deling"
			}
			//toServerInfo.Unlock()
		}
		delete(db.tableMap,key)
		db.Unlock()
		count.DelTable(db.Name,key)
		log.Println("DelTable",db.Name,schemaName,tableName)
	}
	return true
}

func (db *db) AddChannel(Name string,MaxThreadNum int) (*Channel,int) {
	db.Lock()
	db.LastChannelID++
	ChannelID := db.LastChannelID
	if _, ok := db.channelMap[ChannelID]; ok {
		db.Unlock()
		return db.channelMap[ChannelID],ChannelID
	}
	c := NewChannel(MaxThreadNum,Name, db)
	db.channelMap[ChannelID] = c
	ch := count.SetChannel(db.Name,Name)
	db.channelMap[ChannelID].SetFlowCountChan(ch)
	db.Unlock()
	log.Println("AddChannel",db.Name,Name,"MaxThreadNum:",MaxThreadNum)
	return db.channelMap[ChannelID],ChannelID
}

func (db *db) ListChannel() map[int]*Channel {
	db.Lock()
	defer  db.Unlock()
	return db.channelMap
}

func (db *db) GetChannel(channelID int) *Channel {
	if _,ok:=db.channelMap[channelID];!ok{
		return nil
	}
	return db.channelMap[channelID]
}

type Table struct {
	sync.Mutex
	Name         	string
	ChannelKey   	int
	LastToServerID  int
	ToServerList 	[]*ToServer
}
