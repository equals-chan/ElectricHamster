package net.lingala.zip4j;

import jxl.write.WriteException;
import net.lingala.zip4j.exception.ZipException;
import net.lingala.zip4j.model.ZipParameters;
import net.lingala.zip4j.model.enums.AesKeyStrength;
import net.lingala.zip4j.model.enums.EncryptionMethod;
import org.fusesource.jansi.AnsiConsole;

import java.io.File;
import java.io.IOException;
import java.sql.SQLException;
import java.util.ArrayList;

public class control {


    private static void zip(File ZippingFile,String ZippedFilePath,char[] pwd){
        ZipParameters zipParameters = new ZipParameters();
        zipParameters.setEncryptFiles(true);
        zipParameters.setEncryptionMethod(EncryptionMethod.AES);
// Below line is optional. AES 256 is used by default. You can override it to use AES 128. AES 192 is supported only for extracting.
        zipParameters.setAesKeyStrength(AesKeyStrength.KEY_STRENGTH_256);

        ZipFile zipFile = new ZipFile(ZippedFilePath,pwd);
        try {
            zipFile.addFolder(ZippingFile, zipParameters);
        } catch (ZipException e) {
            e.printStackTrace();
        }
    }

    private static String getPwd(int n){
        return RandomPwd.getRandomPwd(n);
    }


    public static void main(String[] args) throws IOException, WriteException, SQLException {
        AnsiConsole.systemInstall();
        properties properties = new properties("./");
        clock clock = new clock();


        //待压缩文件夹最上级目录
        File topFolder = new File(properties.getFolderPath());
        //压缩文件存放处
        String ZippedFilePath = properties.getZippedFilesPath();
        //密码表存放处
        String excelPath = properties.getExcelPath();
        //写excle
        excelWriter excelWriter = new excelWriter("\\passwords.xls",excelPath,properties.getSqlPath());

        int excelOpNum = Integer.parseInt(properties.getExcelOpNum());
        //sql
        sqliteConnecter sqliter = new sqliteConnecter(properties.getSqlPath()+"db.sqlite3");


        //计数器
        counter counter = new counter();
        //bar
        progressBar progressBar = new progressBar();


        ArrayList<ArrayList> temp = new ArrayList();



        File[] filesAndFolders = topFolder.listFiles();
        progressBar.setAllFolderNum(filesAndFolders.length);
        clock.startClock();
        for (File fs : filesAndFolders){
            if (fs.isFile()){ //不压缩文件 只压文件夹
                progressBar.printBar();
                progressBar.addnowNum();
                counter.addThisTimeJumppedNum();
                continue;

            } else {
                //根据分隔符分割路径，集合的最后一项即为文件夹名
                String[] s1 = fs.toString().split("\\\\"); //转义符杀我...

                // 如果该文件夹被压缩过
                String name;
                if (sqliter.isZipped((name = s1[s1.length-1]))){
                    progressBar.printBar();
                    //改进度条
                    progressBar.addnowNum();
                    counter.addThisTimeJumppedNum();
                    System.out.println("\u001b[1;47m 文件夹已被压缩过："+name+"\u001b[0m \n");
                    continue;
                }else {
                    //获取开始时的文件名
                    int FromName = sqliter.getfilesLastNO()+1;

                    ArrayList OneRow = new ArrayList(4);
                    OneRow.add(FromName);
                    //取文件夹名
                    OneRow.add(s1[s1.length-1]);

                    String pwd = getPwd(10); //有风险，不过自用程序，无所谓了

                    OneRow.add(pwd);
                    OneRow.add(times.getNowTimes());
                    //这个暂时没什么用，不过先留着
                    temp.add(OneRow);

                    //文件信息写入
                    sqliter.insert((int)OneRow.get(0),OneRow.get(1).toString(),OneRow.get(2).toString(),OneRow.get(3).toString());
                    //写入excle
                    if (excelOpNum == 1) {
                        excelWriter.AddRow(OneRow.get(0).toString(), OneRow.get(1).toString(), OneRow.get(2).toString(), OneRow.get(3).toString());
                    }

                    //输出进度
                    progressBar.printBar();
                    progressBar.addnowNum();

                    System.out.println("\u001b[1;46m id : "+OneRow.get(0).toString()+"\u001b[0m");
                    System.out.println("\u001b[1;46m 正在压缩： "+OneRow.get(1).toString()+"\u001b[0m \n");

                    //开始压缩
                    zip(fs,ZippedFilePath+"\\"+FromName+".zip",pwd.toCharArray());

                    //文件名+1
                    FromName++;
                    //数据库的文件名+1
                    sqliter.addfilesLastNO();
                    counter.addThisTimeZippedNum();
                }

            }

        }
        sqliter.close();
        //等sqliter结束再调用写入
        if (excelOpNum == 0){
            excelWriter.printAllData();
        }
        excelWriter.push();
        excelWriter.close();

        System.out.println("\u001b[1;45m 程序结束 \u001b[0m");
        counter.printSelf();
        clock.endClock();
        clock.print();
        AnsiConsole.systemUninstall();
    }

}
