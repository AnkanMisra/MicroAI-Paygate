import { Router, Request, Response } from "express";
import { AIService } from "../services/ai";

const router = Router();

router.post("/summarize", async (req: Request, res: Response) => {
  try {
    const { text } = req.body;
    const summary = await AIService.summarize(text, req.correlationId);
    res.json({ result: summary });
  } catch (error: any) {
    res.status(500).json({ error: error.message });
  }
});

router.post("/sentiment", async (req: Request, res: Response) => {
  try {
    const { text } = req.body;
    const sentiment = await AIService.analyzeSentiment(text, req.correlationId);
    res.json({ result: sentiment });
  } catch (error: any) {
    res.status(500).json({ error: error.message });
  }
});

export const aiRoutes = router;
