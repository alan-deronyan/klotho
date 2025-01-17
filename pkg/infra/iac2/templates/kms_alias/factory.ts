import * as aws from '@pulumi/aws'

interface Args {
    Name: string
    AliasName: string
    TargetKey: aws.kms.Key
}

// noinspection JSUnusedLocalSymbols
function create(args: Args): aws.kms.Alias {
    return new aws.kms.Alias(args.Name, {
        targetKeyId: args.TargetKey.id,
        name: args.AliasName,
    })
}
