package database

import (
	"database/sql"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

/*
	CreateDatabase creates tables for node and pod metrics
*/
func CreateDatabase(db *sql.DB) error {
	sqlStmt := `
	create table if not exists nodes (uid text, name text, cpu text, memory text, storage text, time datetime);
	create table if not exists pods (uid text, name text, namespace text, container text, cpu text, memory text, storage text, time datetime);
	`
	_, err := db.Exec(sqlStmt)
	if err != nil {
		return err
	}

	return nil
}

/*
	UpdateDatabase updates nodeMetrics and podMetrics with scraped data
*/
func UpdateDatabase(db *sql.DB, nodeMetrics *v1beta1.NodeMetricsList, podMetrics *v1beta1.PodMetricsList) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare("insert into nodes(uid, name, cpu, memory, storage, time) values(?, ?, ?, ?, ?, datetime('now'))")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, v := range nodeMetrics.Items {
		_, err = stmt.Exec(v.UID, v.Name, v.Usage.Cpu().MilliValue(), v.Usage.Memory().MilliValue()/1000, v.Usage.StorageEphemeral().MilliValue()/1000)
		if err != nil {
			return err
		}
	}

	stmt, err = tx.Prepare("insert into pods(uid, name, namespace, container, cpu, memory, storage, time) values(?, ?, ?, ?, ?, ?, ?, datetime('now'))")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, v := range podMetrics.Items {
		for _, u := range v.Containers {
			_, err = stmt.Exec(v.UID, v.Name, v.Namespace, u.Name, u.Usage.Cpu().MilliValue(), u.Usage.Memory().MilliValue()/1000, u.Usage.StorageEphemeral().MilliValue()/1000)
			if err != nil {
				return err
			}
		}
	}

	err = tx.Commit()

	if err != nil {
		rberr := tx.Rollback()
		if rberr != nil {
			return rberr
		}
		return err
	}

	return nil
}

/*
	CullDatabase deletes rows from nodes and pods based on a time window.
*/
func CullDatabase(db *sql.DB, window *time.Duration) error {
	tx, err := db.Begin()

	windowStr := fmt.Sprintf("-%.0f seconds", window.Seconds())

	nodestmt, err := tx.Prepare("delete from nodes where time <= datetime('now', ?);")
	if err != nil {
		return err
	}

	defer nodestmt.Close()
	res, err := nodestmt.Exec(windowStr)
	if err != nil {
		return err
	}

	affected, _ := res.RowsAffected()
	log.Debugf("Cleaning up nodes: %d rows removed", affected)

	podstmt, err := tx.Prepare("delete from pods where time <= datetime('now', ?);")

	defer podstmt.Close()
	res, err = podstmt.Exec(windowStr)
	if err != nil {
		return err
	}

	affected, _ = res.RowsAffected()
	log.Debugf("Cleaning up pods: %d rows removed", affected)
	err = tx.Commit()

	if err != nil {
		rberr := tx.Rollback()
		if rberr != nil {
			return rberr
		}
		return err
	}

	return nil
}
