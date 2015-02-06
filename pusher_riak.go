package main

import (
	"encoding/json"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/tpjg/goriakpbc"
	"time"
)

// RiakPusher structure will help us to push CDRs to PostgreSQL.
// the structure will held properties to connect to the PG DBMS and
// push the CDRs, such as RiakConnect and RiakBucket
type RiakPusher struct {
	bucket      *riak.Bucket
	RiakConnect string
	RiakBucket  string
	cdrFields   []ParseFields
	switchIP    string
	countPushed int
}

// Init is a constructor for RiakPusher
// It will help setting RiakConnect, cdrFields, switchIP and RiakBucket
func (p *RiakPusher) Init(RiakConnect string, cdrFields []ParseFields, switchIP string, RiakBucket string) {
	p.RiakConnect = RiakConnect
	p.cdrFields = cdrFields
	if switchIP == "" {
		ip, err := externalIP()
		if err == nil {
			switchIP = ip
		}
	}
	p.switchIP = switchIP
	p.RiakBucket = RiakBucket
}

// Connect will help to connect to the DBMS, here we implemented the connection to SQLite
func (p *RiakPusher) Connect() error {
	var err error
	// client := riak.New(p.RiakConnect)
	// err = client.Connect()
	err = riak.ConnectClient(p.RiakConnect)
	if err != nil {
		log.Error("Cannot connect to Riak: ", err.Error())
		return err
	}
	// err = client.Ping()
	// if err != nil {
	// 	log.Error("Cannot ping Riak: ", err.Error())
	// 	return err
	// }
	p.bucket, err = riak.NewBucket("testriak")
	if err != nil {
		log.Error("Cannot connect to Riak Bucket(", p.RiakConnect, "): ", err.Error())
		return err
	}
	return nil
}

// ForceConnect will help to Reconnect to the DBMS
func (p *RiakPusher) ForceConnect() error {
	for {
		err := p.Connect()
		if err != nil {
			log.Error("Error connecting to Riak...", err)
			time.Sleep(time.Second * time.Duration(5))
			continue
		}
		return nil
	}
}

// FmtDataExport will reformat the results properly for import
func (p *RiakPusher) FmtDataExport(fetchedResults map[int][]string) (map[int]map[string]interface{}, error) {
	data := make(map[int]map[string]interface{})
	i := 0
	for _, v := range fetchedResults {
		data[i] = make(map[string]interface{})
		data[i]["id"] = v[0]
		data[i]["switch"] = p.switchIP
		data[i]["callid"] = ""
		// extradata := make(map[string]string)
		for j, f := range p.cdrFields {
			data[i][f.DestField] = v[j+1]
		}
		jsonData, err := json.Marshal(data[i])
		if err != nil {
			log.Error("Error:", err.Error())
			return nil, err
		} else {
			data[i]["jsonfmt"] = string(jsonData)
		}
		i = i + 1
	}
	return data, nil
}

// BatchInsert take care of loop through the fetchedResults and push them to PostgreSQL
func (p *RiakPusher) BatchInsert(fetchedResults map[int][]string) error {
	// create the statement string
	log.WithFields(log.Fields{
		"fetchedResults": fetchedResults,
	}).Debug("Results:")
	var err error
	data, err := p.FmtDataExport(fetchedResults)
	if err != nil {
		return err
	}
	p.countPushed = 0
	for _, val := range data {
		bucketkey := fmt.Sprintf("callid-%v-%v", val["callid"], val["switch"])
		log.Info("New bucketkey=> ", bucketkey)
		obj := p.bucket.NewObject(bucketkey)
		obj.ContentType = "application/json"
		obj.Data = []byte(fmt.Sprintf("%v", val["jsonfmt"]))
		obj.Store()
		// TODO: there isn't a bulk insert mode in Riak, the way to insert faster would be
		// by using connectPool (NewClientPool: Returns a new Client with multiple
		// connections to Riak)
		p.countPushed = p.countPushed + 1
	}
	return nil
}

// Push is the main method that will connect to the DB, create the talbe
// if it doesn't exist and insert all the records received from the Fetcher
func (p *RiakPusher) Push(fetchedResults map[int][]string) error {
	// Connect to DB
	err := p.ForceConnect()
	if err != nil {
		return err
	}
	defer riak.Close()

	// Insert in Batch to DB
	err = p.BatchInsert(fetchedResults)
	if err != nil {
		return err
	}
	log.Info("Total number pushed to Riak:", p.countPushed)
	return nil
}
