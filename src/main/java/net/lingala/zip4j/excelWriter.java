package net.lingala.zip4j;
import java.io.File;
import java.io.IOException;

import jxl.Workbook;
import jxl.write.Label;
import jxl.write.WritableSheet;
import jxl.write.WritableWorkbook;
import jxl.write.WriteException;

/**
 * jxl写Excel
 *
 * @author jianggujin
 *
 */
public class excelWriter{
    private String FileName;
    private String FilePath;
    private File xlsFile;
    private WritableWorkbook workbook;
    private WritableSheet sheet;
    private int pointer=0;

    excelWriter(String fileName,String filePath) throws IOException, WriteException {
        this.FileName = fileName;
        this.FilePath = filePath;
        init();
    }

    private void init() throws IOException, WriteException {
        this.xlsFile = new File(this.FilePath +"\\"+this.FileName);
        this.workbook = Workbook.createWorkbook(xlsFile);
        this.sheet = workbook.createSheet("sheet1", 0);
        writeTitle();
    }

    private void writeTitle() throws WriteException, IOException {
        sheet.addCell(new Label(0,0,"Zip_ID"));
        sheet.addCell(new Label(1,0,"Folder_Name"));
        sheet.addCell(new Label(2,0,"PassWord"));
        sheet.addCell(new Label(3,0,"create_Time"));
        pointer++;

    }

    public void AddRow(String ... strings) throws IOException,WriteException{
        int i = 0;
        for (String s : strings){
            sheet.addCell(new Label(i,pointer,s));
            i++;
        }
        pointer++;

    }

    public void close() throws IOException, WriteException {
        workbook.write();
        workbook.close();
    }



}