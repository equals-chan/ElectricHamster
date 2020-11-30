package net.lingala.zip4j;

public class progressBar {
    private int allFolderNum;
    private double nowNum=0;

    public int getAllFolderNum() {
        return allFolderNum;
    }

    public double getNowNum() {
        return nowNum;
    }

    public void printBar(){
        System.out.printf("进度：%.2f%%\n",(nowNum/allFolderNum)*100);
    }

    public void addnowNum(){
        nowNum+=1;
    }

    public void setAllFolderNum(int allFolderNum) {
        this.allFolderNum = allFolderNum;
    }


}
