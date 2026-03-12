package main

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// IMPORTANT: Replace "my-org" with your actual Pulumi Organization name
		// If you are using an individual account, it's your Pulumi username.
		orgName := "schitiz-datazip-io"
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

		// 4. Create an EC2 Instance in the shared subnet using the shared SG
		instance, err := ec2.NewInstance(ctx, "tenant-a-ec2", &ec2.InstanceArgs{
			InstanceType:        pulumi.String("t3.micro"),
			VpcSecurityGroupIds: pulumi.StringArray{sgId},
			SubnetId:            subnetId,
			Ami:                 pulumi.String(ami.Id),
			Tags: pulumi.StringMap{
				"Name": pulumi.String("tenant-a-ec2"),
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
