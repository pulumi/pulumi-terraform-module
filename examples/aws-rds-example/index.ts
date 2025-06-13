import * as vpcmod from '@pulumi/vpcmod';
import * as pulumi from '@pulumi/pulumi';
import * as rdsmod from '@pulumi/rdsmod';
import * as aws from '@pulumi/aws';
import * as std from '@pulumi/std';

const azs = aws.getAvailabilityZonesOutput({
  filters: [{
        name: "opt-in-status",
        values: ["opt-in-not-required"],
    }]
}).names.apply(names => names.slice(0, 3));

const cidr = "10.0.0.0/16";

const cfg = new pulumi.Config();
const prefix = cfg.get("prefix") ?? pulumi.getStack();

const vpc = new vpcmod.Module("test-vpc", {
  azs: azs,
  name: `test-vpc-${prefix}`,
  cidr,
  public_subnets: azs.apply(azs => azs.map((_, i) => {
    return getCidrSubnet(cidr, i+1);
  })),
  private_subnets: azs.apply(azs => azs.map((_, i) => {
    return getCidrSubnet(cidr, i+1+4);
  })),
  database_subnets: azs.apply(azs => azs.map((_, i) => {
    return getCidrSubnet(cidr, i+1 + 8);
  })),

  create_database_subnet_group: true,
});

const rdsSecurityGroup = new aws.ec2.SecurityGroup('test-rds-sg', {
  vpcId: vpc.vpc_id.apply(id => id!),
});

new aws.vpc.SecurityGroupIngressRule('test-rds-sg-ingress', {
  ipProtocol: 'tcp',
  securityGroupId: rdsSecurityGroup.id,
  cidrIpv4: vpc.vpc_cidr_block.apply(cidr => cidr!),
  fromPort: 3306,
  toPort: 3306,
});

new rdsmod.Module("test-rds", {
  engine: "mysql",
  identifier: `test-rds-${prefix}`,
  manage_master_user_password: true,
  publicly_accessible: false,
  allocated_storage: 20,
  max_allocated_storage: 100,
  instance_class: "db.t4g.micro",
  engine_version: "8.0",
  family: "mysql8.0",
  db_name: "completeMysql",
  username: "complete_mysql",
  port: '3306',
  multi_az: true,
  db_subnet_group_name: vpc.database_subnet_group_name.apply(name => name!),
  vpc_security_group_ids: [rdsSecurityGroup.id],
  skip_final_snapshot: true,
  deletion_protection: false,
  create_db_option_group: false,
  create_db_parameter_group: false,

})

function getCidrSubnet(cidr: string, netnum: number): pulumi.Output<string> {
    return std.cidrsubnetOutput({
    input: cidr,
    newbits: 8,
    netnum,
  }).result
}
