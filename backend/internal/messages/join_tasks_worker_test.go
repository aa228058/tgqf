package messages

import (
    "testing"
    "time"
)

func TestJoinTaskCooldownSecondsAddsBufferAfterCeiling(t *testing.T) {
    cases:=[]struct{name string;wait time.Duration;want int64}{
        {"exact seconds",2*time.Minute,121},
        {"fractional second",2*time.Minute+100*time.Millisecond,122},
        {"single second",time.Second,2},
        {"zero",0,0},
        {"negative",-time.Second,0},
    }
    for _,tc:=range cases {
        t.Run(tc.name,func(t *testing.T){
            if got:=joinTaskCooldownSeconds(tc.wait);got!=tc.want {t.Fatalf("joinTaskCooldownSeconds(%s)=%d, want %d",tc.wait,got,tc.want)}
        })
    }
}

func TestJoinTaskRetryDelayIsBoundedExponential(t *testing.T) {
    cases := []struct { attempt int; want time.Duration }{
        {0,30*time.Second},
        {1,30*time.Second},
        {2,60*time.Second},
        {3,2*time.Minute},
        {4,4*time.Minute},
        {5,8*time.Minute},
        {6,16*time.Minute},
        {7,30*time.Minute},
        {20,30*time.Minute},
    }
    for _, tc := range cases {
        if got := joinTaskRetryDelay(tc.attempt); got != tc.want {
            t.Fatalf("joinTaskRetryDelay(%d)=%s, want %s", tc.attempt, got, tc.want)
        }
    }
}
