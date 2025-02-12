import { Handler } from 'aws-lambda';

export const handler: Handler = async (event, context) => {
    console.log('Hello with <3 from Pulumi!');
    return context.logStreamName;
};