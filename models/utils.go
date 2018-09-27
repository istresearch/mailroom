package models

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/juju/errors"
	"github.com/sirupsen/logrus"
)

func readJSONRow(rows *sqlx.Rows, destination interface{}) error {
	var jsonBlob string
	err := rows.Scan(&jsonBlob)
	if err != nil {
		return errors.Annotate(err, "error scanning row json")
	}

	err = json.Unmarshal([]byte(jsonBlob), destination)
	if err != nil {
		return errors.Annotate(err, "error unmarshalling row json")
	}

	return nil
}

// extractValues is just a simple utility method that extracts the portion between `VALUE(`
// and `)` in the passed in string. (leaving VALUE but not the parentheses)
func extractValues(sql string) (string, error) {
	startValues := strings.Index(sql, "VALUES(")
	if startValues <= 0 {
		return "", errors.Errorf("unable to find VALUES( in bulk insert SQL: %s", sql)
	}

	// find the matching end parentheses, we need to count balances here
	openCount := 1
	endValues := -1
	for i, r := range sql[startValues+7:] {
		if r == '(' {
			openCount++
		} else if r == ')' {
			openCount--
			if openCount == 0 {
				endValues = i + startValues + 7
				break
			}
		}
	}

	if endValues <= 0 {
		return "", errors.Errorf("unable to find end of VALUES() in bulk insert sql: %s", sql)
	}

	return sql[startValues+6 : endValues+1], nil
}

func BulkSQL(ctx context.Context, label string, tx *sqlx.Tx, sql string, vs []interface{}) error {
	// no values, nothing to do
	if len(vs) == 0 {
		return nil
	}

	start := time.Now()

	// this will be our SQL placeholders for values in our final query, built dynamically
	values := strings.Builder{}
	values.Grow(7 * len(vs))

	// this will be each of the arguments to match the positional values above
	args := make([]interface{}, 0, len(vs)*5)

	// for each value we build a bound SQL statement, then extract the values clause
	for i, value := range vs {
		valueSQL, valueArgs, err := sqlx.Named(sql, value)
		if err != nil {
			return errors.Annotatef(err, "error converting bulk insert args")
		}

		args = append(args, valueArgs...)
		argValues, err := extractValues(valueSQL)
		if err != nil {
			return errors.Annotatef(err, "error extracting values from sql: %s", valueSQL)
		}

		// append to our global values, adding comma if necessary
		values.WriteString(argValues)
		if i+1 < len(vs) {
			values.WriteString(",")
		}
	}

	valuesSQL, err := extractValues(sql)
	if err != nil {
		return errors.Annotatef(err, "error extracting values from sql: %s", sql)
	}

	bulkInsert := tx.Rebind(strings.Replace(sql, valuesSQL, values.String(), -1))

	// insert them all at once
	rows, err := tx.QueryxContext(ctx, bulkInsert, args...)
	if err != nil {
		return errors.Annotatef(err, "error during bulk insert")
	}
	defer rows.Close()

	// read in all our inserted rows, scanning in any values if we have a returning clause
	if strings.Contains(strings.ToUpper(sql), "RETURNING") {
		for _, v := range vs {
			if !rows.Next() {
				return errors.Errorf("did not receive expected number of rows on insert")
			}

			err = rows.StructScan(v)
			if err != nil {
				return errors.Annotate(err, "error scanning for insert id")
			}
		}
	}

	logrus.WithField("elapsed", time.Since(start)).WithField("rows", len(vs)).Debugf("%s bulk insert complete", label)

	return nil
}
