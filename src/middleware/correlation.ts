import { Request, Response, NextFunction } from 'express';
import { v4 as uuidv4 } from 'uuid';

// Extend the Express Request type to include our custom property
declare global {
  namespace Express {
    interface Request {
      correlationId?: string;
    }
  }
}

export const correlationMiddleware = (req: Request, res: Response, next: NextFunction) => {
  // 1. Check for existing ID (headers are lowercase in Express)
  const existingId = req.headers['x-correlation-id'];

  // 2. Use existing or generate new
  const correlationId = Array.isArray(existingId) 
    ? existingId[0] 
    : (existingId || uuidv4());

  // 3. Attach to request object for other services to use
  req.correlationId = correlationId;

  // 4. Send it back in the response headers
  res.setHeader('X-Correlation-ID', correlationId);

  // 5. Log it so we can see it working
  console.log(`[CorrelationID: ${correlationId}] ${req.method} ${req.path}`);

  next();
};

