package net.lingala.zip4j;

import jxl.write.WriteException;

import java.io.IOException;

public class test1 {
    public static void main(String[] args) throws IOException, WriteException {

        excelWriter excelWriter = new excelWriter("passwords.xls","./");
        try {
            excelWriter.AddRow("sadasd","sfkjfhdgfejasd","fhjdsaghfd","备注");
            excelWriter.close();
        } catch (IOException e) {
            e.printStackTrace();
        } catch (WriteException e) {
            e.printStackTrace();
        }

    }
}
