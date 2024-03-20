// Get/set arbitrary flags in the Weave Net persistence DB
package main

import (
	"fmt"

	"github.com/rajch/weave/db"
)

const dbFlagPrefix = "flag:"

func getDBFlag(args []string) error {
	if len(args) != 2 {
		cmdUsage("check-db-flag", "<db-prefix> <flag-name>")
	}
	dbPrefix := args[0]
	flagName := args[1]

	d, err := db.NewBoltDBReadOnly(dbPrefix)
	if err != nil {
		return err
	}
	defer d.Close()
	var value string
	nameFound, err := d.Load(dbFlagPrefix+flagName, &value)
	if err != nil {
		return err
	}
	if !nameFound {
		return fmt.Errorf("")
	}
	fmt.Print(value)
	return nil
}

func setDBFlag(args []string) error {
	if len(args) != 3 {
		cmdUsage("set-db-flag", "<db-prefix> <flag-name> <flag-value>")
	}
	dbPrefix := args[0]
	flagName := args[1]
	value := args[2]

	d, err := db.NewBoltDB(dbPrefix)
	if err != nil {
		return err
	}
	defer d.Close()
	err = d.Save(dbFlagPrefix+flagName, value)
	if err != nil {
		return err
	}
	return nil
}
