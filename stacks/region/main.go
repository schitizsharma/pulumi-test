package main

import (
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/rds"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// Create a new VPC
		vpc, err := ec2.NewVpc(ctx, "my-deployment-vpc", &ec2.VpcArgs{
			CidrBlock:          pulumi.String("10.0.0.0/16"),
			EnableDnsHostnames: pulumi.Bool(true),
			EnableDnsSupport:   pulumi.Bool(true),
			Tags: pulumi.StringMap{
				"Name": pulumi.String("my-deployment-vpc"),
			},
		})
		if err != nil {
			return err
		}

		// Create an Internet Gateway
		igw, err := ec2.NewInternetGateway(ctx, "my-deployment-igw", &ec2.InternetGatewayArgs{
			VpcId: vpc.ID(),
			Tags: pulumi.StringMap{
				"Name": pulumi.String("my-deployment-igw"),
			},
		})
		if err != nil {
			return err
		}

		// Fetch available Availability Zones
		azs, err := aws.GetAvailabilityZones(ctx, &aws.GetAvailabilityZonesArgs{
			State: pulumi.StringRef("available"),
		}, nil)
		if err != nil {
			return err
		}

		// Create a Public Subnet
		publicSubnet, err := ec2.NewSubnet(ctx, "my-deployment-public-subnet", &ec2.SubnetArgs{
			VpcId:               vpc.ID(),
			CidrBlock:           pulumi.String("10.0.1.0/24"),
			AvailabilityZone:    pulumi.String(azs.Names[0]),
			MapPublicIpOnLaunch: pulumi.Bool(true),
			Tags: pulumi.StringMap{
				"Name": pulumi.String("my-deployment-public-subnet"),
			},
		})
		if err != nil {
			return err
		}

		// Create a Route Table for Public Subnet
		publicRt, err := ec2.NewRouteTable(ctx, "my-deployment-public-rt", &ec2.RouteTableArgs{
			VpcId: vpc.ID(),
			Routes: ec2.RouteTableRouteArray{
				&ec2.RouteTableRouteArgs{
					CidrBlock: pulumi.String("0.0.0.0/0"),
					GatewayId: igw.ID(),
				},
			},
			Tags: pulumi.StringMap{
				"Name": pulumi.String("my-deployment-public-rt"),
			},
		})
		if err != nil {
			return err
		}

		_, err = ec2.NewRouteTableAssociation(ctx, "my-deployment-public-rta", &ec2.RouteTableAssociationArgs{
			SubnetId:     publicSubnet.ID(),
			RouteTableId: publicRt.ID(),
		})
		if err != nil {
			return err
		}

		// RDS DB Subnet Group requires at least 2 subnets in different AZs
		privateSubnet1, err := ec2.NewSubnet(ctx, "my-deployment-private-subnet-1", &ec2.SubnetArgs{
			VpcId:            vpc.ID(),
			CidrBlock:        pulumi.String("10.0.2.0/24"),
			AvailabilityZone: pulumi.String(azs.Names[0]),
			Tags: pulumi.StringMap{
				"Name": pulumi.String("my-deployment-private-subnet-1"),
			},
		})
		if err != nil {
			return err
		}

		privateSubnet2, err := ec2.NewSubnet(ctx, "my-deployment-private-subnet-2", &ec2.SubnetArgs{
			VpcId:            vpc.ID(),
			CidrBlock:        pulumi.String("10.0.3.0/24"),
			AvailabilityZone: pulumi.String(azs.Names[1]),
			Tags: pulumi.StringMap{
				"Name": pulumi.String("my-deployment-private-subnet-2"),
			},
		})
		if err != nil {
			return err
		}

		// Create DB Subnet Group
		dbSubnetGroup, err := rds.NewSubnetGroup(ctx, "my-deployment-db-subnet-group", &rds.SubnetGroupArgs{
			SubnetIds: pulumi.StringArray{
				privateSubnet1.ID(),
				privateSubnet2.ID(),
			},
			Tags: pulumi.StringMap{
				"Name": pulumi.String("my-deployment-db-subnet-group"),
			},
		})
		if err != nil {
			return err
		}

		// Security Group for EC2 that allows SSH and HTTP
		ec2Sg, err := ec2.NewSecurityGroup(ctx, "my-deployment-ec2-sg", &ec2.SecurityGroupArgs{
			VpcId:       vpc.ID(),
			Description: pulumi.String("Allow SSH and HTTP for EC2"),
			Ingress: ec2.SecurityGroupIngressArray{
				&ec2.SecurityGroupIngressArgs{
					Protocol:   pulumi.String("tcp"),
					FromPort:   pulumi.Int(22),
					ToPort:     pulumi.Int(22),
					CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
				},
				&ec2.SecurityGroupIngressArgs{
					Protocol:   pulumi.String("tcp"),
					FromPort:   pulumi.Int(80),
					ToPort:     pulumi.Int(80),
					CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
				},
			},
			Egress: ec2.SecurityGroupEgressArray{
				&ec2.SecurityGroupEgressArgs{
					Protocol:   pulumi.String("-1"),
					FromPort:   pulumi.Int(0),
					ToPort:     pulumi.Int(0),
					CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
				},
			},
		})
		if err != nil {
			return err
		}

		// Security Group for RDS that allows Postgres traffic from within the VPC
		dbSg, err := ec2.NewSecurityGroup(ctx, "my-deployment-db-sg", &ec2.SecurityGroupArgs{
			VpcId:       vpc.ID(),
			Description: pulumi.String("Allow Postgres from VPC"),
			Ingress: ec2.SecurityGroupIngressArray{
				&ec2.SecurityGroupIngressArgs{
					Protocol:   pulumi.String("tcp"),
					FromPort:   pulumi.Int(5432),
					ToPort:     pulumi.Int(5432),
					CidrBlocks: pulumi.StringArray{vpc.CidrBlock},
				},
			},
			Egress: ec2.SecurityGroupEgressArray{
				&ec2.SecurityGroupEgressArgs{
					Protocol:   pulumi.String("-1"),
					FromPort:   pulumi.Int(0),
					ToPort:     pulumi.Int(0),
					CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
				},
			},
		})
		if err != nil {
			return err
		}

		// Export the IDs
		ctx.Export("vpcId", vpc.ID())
		ctx.Export("subnetId", publicSubnet.ID())
		ctx.Export("securityGroupId", ec2Sg.ID())
		ctx.Export("dbSubnetGroupName", dbSubnetGroup.Name)
		ctx.Export("dbSecurityGroupId", dbSg.ID())

		return nil
	})
}
