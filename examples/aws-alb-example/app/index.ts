import { ALBHandler, ALBResult } from 'aws-lambda';

export const handler: ALBHandler = async (event, context): Promise<ALBResult> => {
  return {
    statusCode: 200,
    body: 'Hello, world!',
  };
}
