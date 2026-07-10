package handlers

import (
    "fmt"
    "net/http"
    "os"
    "strconv"

    "github.com/gin-gonic/gin"
)

type SummarizeRequest struct {
    Text string `json:"text" binding:"required"`
}

func Summarize(c *gin.Context) {
    var req SummarizeRequest

    if err := c.ShouldBindJSON(&req); err != nil {
        if err.Error() == "http: request body too large" {
            c.JSON(http.StatusRequestEntityTooLarge, gin.H{
                "error":       "body_too_large",
                "message":     "Request body exceeds the maximum allowed size.",
                "max_size_kb": 32,
            })
            return
        }
        c.JSON(http.StatusBadRequest, gin.H{
            "error":   "invalid_request",
            "message": err.Error(),
        })
        return
    }

    maxLen := getMaxTextLength()

    if len(req.Text) > maxLen {
        c.JSON(http.StatusRequestEntityTooLarge, gin.H{
            "error":      "text_too_long",
            "message":    fmt.Sprintf("Text must be %d characters or fewer.", maxLen),
            "max_length": maxLen,
            "received":   len(req.Text),
        })
        return
    }

    if len(req.Text) == 0 {
        c.JSON(http.StatusBadRequest, gin.H{
            "error":   "text_empty",
            "message": "The 'text' field cannot be empty.",
        })
        return
    }

    // Existing handler logic below
}

func getMaxTextLength() int {
    const defaultMaxLen = 8000
    valStr := os.Getenv("MAX_SUMMARIZE_TEXT_LENGTH")
    if valStr == "" {
        return defaultMaxLen
    }
    val, err := strconv.Atoi(valStr)
    if err != nil || val <= 0 {
        return defaultMaxLen
    }
    return val
}