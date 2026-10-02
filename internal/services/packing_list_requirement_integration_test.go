package services

import (
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"

	_ "github.com/lib/pq"
)

func TestPackingListRetainsManualDemandAndExcludesArchives(t *testing.T) {
	dsn := os.Getenv("WAREHOUSE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("disposable _test PostgreSQL required")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(u.Path, "_test") {
		t.Fatal("dedicated _test database required")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	exec := func(q string) {
		t.Helper()
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	const ns = "packing_requirement_lifecycle_test"
	exec(`DROP SCHEMA IF EXISTS ` + ns + ` CASCADE;CREATE SCHEMA ` + ns + `;SET search_path TO ` + ns)
	defer db.Exec(`DROP SCHEMA IF EXISTS ` + ns + ` CASCADE`)
	exec(`CREATE TABLE customers(customerid INT PRIMARY KEY,firstname TEXT,lastname TEXT);INSERT INTO customers VALUES(1,'Test','Customer');
CREATE TABLE jobs(jobid INT PRIMARY KEY,job_code TEXT,description TEXT,customerid INT,startdate DATE,enddate DATE,deleted_at TIMESTAMP);INSERT INTO jobs(jobid,job_code,description,customerid) VALUES(1,'JOB001','Requirement test',1);
CREATE TABLE products(productid INT PRIMARY KEY,name TEXT,is_accessory BOOLEAN,lifecycle_status TEXT);INSERT INTO products VALUES(1,'Position plus manual',false,'active'),(2,'Manual only',false,'active'),(3,'Archived material',false,'active'),(4,'Included accessory',true,'active');
CREATE TABLE job_positions(job_id INT,product_id INT,position_type TEXT,quantity NUMERIC,unit TEXT,sort_order INT);INSERT INTO job_positions VALUES(1,1,'product',3,'Stück',1);
CREATE TABLE job_product_requirements(id INT PRIMARY KEY,job_id INT,product_id INT,quantity INT,manual_quantity INT,deleted_at TIMESTAMPTZ);INSERT INTO job_product_requirements VALUES(1,1,1,5,2,NULL),(2,1,2,4,4,NULL),(3,1,3,7,7,NOW());
CREATE TABLE product_dependencies(product_id INT,dependency_product_id INT,default_quantity NUMERIC,lifecycle_status TEXT);INSERT INTO product_dependencies VALUES(1,4,2,'active');`)
	list, err := LoadPackingList(db, 1)
	if err != nil {
		t.Fatal(err)
	}
	quantities := map[int]float64{}
	for _, item := range list.Items {
		quantities[item.ProductID] += item.Quantity
	}
	if quantities[1] != 5 || quantities[2] != 4 || quantities[3] != 0 || quantities[4] != 10 || len(list.Items) != 3 {
		t.Fatal("wrong combined/archived demand", list.Items)
	}
	exec(`UPDATE job_product_requirements SET deleted_at=NOW() WHERE product_id=2`)
	list, err = LoadPackingList(db, 1)
	if err != nil || len(list.Items) != 2 {
		t.Fatal("archive remained in packing list", list, err)
	}
	exec(`UPDATE job_product_requirements SET deleted_at=NULL WHERE product_id=2`)
	list, err = LoadPackingList(db, 1)
	if err != nil || len(list.Items) != 3 {
		t.Fatal("restore lost packing demand", list, err)
	}
}
