import { Handler } from 'aws-lambda';

export const handler: Handler = async (event, context) => {
    console.log('I hate AWS!');
    return context.logStreamName;
};