package main

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/rds"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"

	_ "github.com/lib/pq"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// IMPORTANT: Replace "my-org" with your actual Pulumi Organization name
		// If you are using an individual account, it's your Pulumi username.
		orgName := "schitiz-datazip-io-org"
		regionStackName := fmt.Sprintf("%s/olake-region-platform/dev", orgName)

		// 1. Reference the Region Stack
		regionStack, err := pulumi.NewStackReference(ctx, regionStackName, nil)
		if err != nil {
			return fmt.Errorf("failed to reference region stack: %w", err)
		}

		// 2. Get Outputs from the Region Stack
		subnetId := regionStack.GetStringOutput(pulumi.String("subnetId"))
		ec2SgId := regionStack.GetStringOutput(pulumi.String("securityGroupId"))
		dbSubnetGroupName := regionStack.GetStringOutput(pulumi.String("dbSubnetGroupName"))
		dbSgId := regionStack.GetStringOutput(pulumi.String("dbSecurityGroupId"))

		// 3. Find the latest Amazon Linux 2023 AMI
		ami, err := ec2.LookupAmi(ctx, &ec2.LookupAmiArgs{
			MostRecent: pulumi.BoolRef(true),
			Owners:     []string{"amazon"},
			Filters: []ec2.GetAmiFilter{
				{
					Name:   "name",
					Values: []string{"al2023-ami-2023.*-x86_64"},
				},
				{
					Name:   "architecture",
					Values: []string{"x86_64"},
				},
			},
		})
		if err != nil {
			return fmt.Errorf("failed to lookup AMI: %w", err)
		}

		// 4. Dynamically name the instance based on Pulumi Config or default to stack name
		tenantName := ctx.Stack()

		// Attempt to get instanceName from config, fallback to stack name if not set
		conf := config.New(ctx, "")
		instanceName := conf.Get("instanceName")
		if instanceName == "" {
			instanceName = fmt.Sprintf("%s-ec2", tenantName)
		}

		// 5. Create an EC2 Instance in the shared subnet using the shared SG
		instance, err := ec2.NewInstance(ctx, instanceName, &ec2.InstanceArgs{
			InstanceType:        pulumi.String("t3.micro"),
			VpcSecurityGroupIds: pulumi.StringArray{ec2SgId},
			SubnetId:            subnetId,
			Ami:                 pulumi.String(ami.Id),
			Tags: pulumi.StringMap{
				"Name":   pulumi.String(instanceName),
				"Tenant": pulumi.String(tenantName),
			},
		})
		if err != nil {
			return fmt.Errorf("failed to create instance: %w", err)
		}

		// 6. Create an RDS Postgres Instance in the DB Subnet Group
		dbName := "tenantdb"
		dbUser := "postgres"
		dbPass := "postgres123"

		db, err := rds.NewInstance(ctx, fmt.Sprintf("%s-db", tenantName), &rds.InstanceArgs{
			Engine:              pulumi.String("postgres"),
			EngineVersion:       pulumi.String("14"),
			InstanceClass:       pulumi.String("db.t3.micro"),
			AllocatedStorage:    pulumi.Int(20),
			DbSubnetGroupName:   dbSubnetGroupName,
			VpcSecurityGroupIds: pulumi.StringArray{dbSgId},
			DbName:              pulumi.String(dbName),
			Username:            pulumi.String(dbUser),
			Password:            pulumi.String(dbPass),
			SkipFinalSnapshot:   pulumi.Bool(true),
			PubliclyAccessible:  pulumi.Bool(false),
			Tags: pulumi.StringMap{
				"Tenant": pulumi.String(tenantName),
			},
		})
		if err != nil {
			return fmt.Errorf("failed to create db instance: %w", err)
		}

		// 7. Connect to Postgres and run query from the Pulumi Go context
		dbConnectionTest := db.Address.ApplyT(func(address string) (string, error) {
			if ctx.DryRun() {
				fmt.Println("Dry run - skipping DB connection")
				return "Dry run - skipped", nil
			}

			// The agent running this code must be inside the VPC to reach this private RDS address
			connStr := fmt.Sprintf("postgres://%s:%s@%s:5432/%s?sslmode=disable", dbUser, dbPass, address, dbName)

			var dbConn *sql.DB
			var dbErr error

			fmt.Printf("Attempting to connect to database at %s...\n", address)

			// Wait for DB to truly be ready for connections (RDS sometimes takes a moment after "available")
			for i := 0; i < 15; i++ { // wait up to ~2.5 mins
				dbConn, dbErr = sql.Open("postgres", connStr)
				if dbErr == nil {
					dbErr = dbConn.Ping()
					if dbErr == nil {
						break
					}
				}
				if dbConn != nil {
					dbConn.Close()
				}
				time.Sleep(10 * time.Second)
			}
			if dbErr != nil {
				return "", fmt.Errorf("failed to connect to db after retries: %v", dbErr)
			}
			defer dbConn.Close()

			var version string
			err := dbConn.QueryRow("SELECT version()").Scan(&version)
			if err != nil {
				return "", fmt.Errorf("failed to query version: %v", err)
			}

			msg := fmt.Sprintf("Successfully connected to RDS! Postgres Version: %s", version)
			fmt.Println(msg)

			return msg, nil
		})

		// Export Outputs
		ctx.Export("tenantPublicIp", instance.PublicIp)
		ctx.Export("tenantDbAddress", db.Address)
		ctx.Export("dbConnectionTest", dbConnectionTest)

		return nil
	})
}
