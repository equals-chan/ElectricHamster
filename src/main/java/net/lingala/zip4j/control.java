package net.lingala.zip4j;

import jxl.write.WriteException;
import net.lingala.zip4j.exception.ZipException;
import net.lingala.zip4j.model.ZipParameters;
import net.lingala.zip4j.model.enums.AesKeyStrength;
import net.lingala.zip4j.model.enums.EncryptionMethod;

import java.io.File;
import java.io.IOException;
import java.sql.SQLException;
import java.util.ArrayList;
import java.util.List;

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
        //最上级目录
        File topFolder = new File("D:\\Codes\\ElectricHamster\\resources\\test1");
        //压缩文件存放处
        String ZippedFilePath = "./";
        //密码表存放处
        String excelPath = "./";
        //文件名开始处
        int FromName = 10001;


        excelWriter excelWriter = new excelWriter("\\passwords.xls",excelPath);
        ArrayList<ArrayList> temp = new ArrayList();

        sqliteConnecter sqliter = new sqliteConnecter();

        File[] filesAndFolders = topFolder.listFiles();


        for (File fs : filesAndFolders){
            if (fs.isFile()){
                continue;
            }else {
                String[] s1 = fs.toString().split("\\\\"); //转义符杀我...
                ArrayList OneRow = new ArrayList(3);
                OneRow.add(FromName);
                OneRow.add(s1[s1.length-1]);
                String pwd = getPwd(10); //有风险，不过自用程序，无所谓了
                OneRow.add(pwd);
                temp.add(OneRow);
                sqliter.insert((int)OneRow.get(0),OneRow.get(1).toString(),OneRow.get(2).toString());
                excelWriter.AddRow(OneRow.get(0).toString(),OneRow.get(1).toString(),OneRow.get(2).toString());
                zip(fs,ZippedFilePath+"\\"+FromName+".zip",pwd.toCharArray());
                FromName++;
            }

        }
        sqliter.close();
        excelWriter.close();

    }

}
