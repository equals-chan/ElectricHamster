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
        System.out.println("\u001b[1;42m 压缩了 "+thisTimeZippedNum+" 个文件夹\n跳过了已经压缩过的 "+thisTimeJumppedNum +" 个文件夹\u001b[0m");
    }
}
