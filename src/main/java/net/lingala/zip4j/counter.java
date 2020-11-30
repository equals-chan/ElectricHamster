package net.lingala.zip4j;

public class counter {
    private int thisTimeZippedNum=0;
    private int thisTimeJumppedNum=0;

    public int getThisTimeJumppedNum() {
        return thisTimeJumppedNum;
    }

    public int getThisTimeZippedNum() {
        return thisTimeZippedNum;
    }

    public void addThisTimeJumppedNum() {
        this.thisTimeJumppedNum += 1;
    }

    public void addThisTimeZippedNum() {
        this.thisTimeZippedNum += 1;
    }

    public void printSelf(){
        System.out.printf("压缩了 %d 个文件夹 \n跳过了已经压缩过的 %d 个文件夹",thisTimeZippedNum,thisTimeJumppedNum);
    }
}
