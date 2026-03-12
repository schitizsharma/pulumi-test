package main

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
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
		sgId := regionStack.GetStringOutput(pulumi.String("securityGroupId"))

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
			VpcSecurityGroupIds: pulumi.StringArray{sgId},
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

		// Export the Instance's Public IP
		ctx.Export("tenantPublicIp", instance.PublicIp)

		return nil
	})
}
