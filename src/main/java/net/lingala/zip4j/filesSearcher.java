package net.lingala.zip4j;

import java.io.File;
import java.util.ArrayList;
import java.util.List;

public class filesSearcher {

    private File filePath;
    private List<File> allFilesNeedToZip;

    public filesSearcher(String path){
        this.filePath = new File(path);
    }

    public List<File> getAllFilesInThisPath(){
        allFilesNeedToZip = new ArrayList();
        File[] files = filePath.listFiles();
        DFS(files);

        System.out.println(allFilesNeedToZip.toString());
        return allFilesNeedToZip;
    }

    public void DFS(File[] FilePAth){
        for (int i = 0; i < FilePAth.length; i++) {
            if (FilePAth[i].isFile()) {//是文件
                allFilesNeedToZip.add(FilePAth[i]);
            }else if (FilePAth[i].isDirectory()){
                //allFilesNeedToZip.add(FilePAth[i]);
                DFS(FilePAth[i].listFiles());
            }

        }

    }


    public static void main(String[] args) {
        File file = new File("D:\\Codes\\zip4j\\resources\\test1");
        for (File s : file.listFiles()){
            if (s.isDirectory()){
                System.out.println(s.toString());
                String[] s1 = s.toString().split("\\\\"); //转义符杀我...
                System.out.printf("length:%d  ",s1.length);
                System.out.println(s1[s1.length-1]);
            }
        }
    }


}
