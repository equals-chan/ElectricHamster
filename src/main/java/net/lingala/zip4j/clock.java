package net.lingala.zip4j;

public class clock {

    private long startTime = 0;
    private long endTime = 0;

    private long getTime(){
        return System.currentTimeMillis();
    }

    public void startClock(){
        this.startTime = getTime()/1000;
    }

    public void endClock(){
        this.endTime = getTime()/1000;
    }

    public long getClock(){
        return endTime - startTime;
    }

    public void clearClock(){
        this.startTime = 0;
        this.endTime = 0;
    }

    public void print(){
        long times = getClock();
        long minute = times/60;
        long sec = times%60;
        System.out.println("\u001b[1m 共计用时 "+minute+ "分钟 "+sec+ "秒\u001b[0m");
    }


}
